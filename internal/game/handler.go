// Package game drives a single client connection through the protocol: the
// version handshake, session authentication against rom-api, and dispatch of
// subsequent messages to the world. It is the seam where wire messages become
// game actions.
package game

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/octrys/rom-server-poc/internal/auth"
	"github.com/octrys/rom-server-poc/internal/persist"
	"github.com/octrys/rom-server-poc/internal/protocol"
	"github.com/octrys/rom-server-poc/internal/transport"
	"github.com/octrys/rom-server-poc/internal/world"
)

// Handler carries the shared dependencies for serving connections. One Handler
// is shared across all connections; per-connection state lives in session.
type Handler struct {
	Auth   *auth.Client
	Store  persist.Store
	World  *world.World
	Logger *slog.Logger
	// ProtocolVer, when non-zero, rejects clients that report a different
	// version in C2S_VersionCheck; zero accepts any.
	ProtocolVer int32
	// WorldID is the world this server hosts; a session's world must match.
	WorldID int
}

// session is the mutable per-connection state.
type session struct {
	conn        *transport.Conn
	accountID   int64
	accountCode string
	authed      bool
	startedAt   time.Time
}

// Serve runs the connection's lifecycle until it closes or ctx is cancelled. The
// handshake has already been performed on conn.
func (h *Handler) Serve(ctx context.Context, conn *transport.Conn) {
	s := &session{conn: conn, startedAt: time.Now()}
	log := h.Logger.With("peer", conn.RemoteAddr().String())
	log.Info("connection established")

	defer func() {
		if s.authed {
			h.World.Leave(ctx, s.accountID)
		}
		_ = conn.Close()
		log.Info("connection closed")
	}()

	for {
		if ctx.Err() != nil {
			return
		}
		msg, values, tail, err := conn.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				log.Warn("recv failed", "error", err)
			}
			return
		}
		if msg == nil {
			log.Warn("unknown opcode; ignoring")
			continue
		}
		log.Debug("recv", "message", msg.Name)
		if err := h.dispatch(ctx, s, msg, values); err != nil {
			log.Warn("handler failed", "message", msg.Name, "error", err)
			return
		}
		_ = tail // opaque remainder for not-yet-decoded fields; ignored for now
	}
}

// dispatch routes one decoded message. Unhandled messages are logged so the
// missing handlers are visible as the client exercises the flow.
func (h *Handler) dispatch(ctx context.Context, s *session, msg *protocol.Message, values protocol.Values) error {
	switch msg.Name {
	case "C2S_VersionCheck":
		return h.handleVersionCheck(s, values)
	case "C2S_SessionAuthLogin":
		return h.handleLogin(ctx, s, values)
	}

	// Everything past login requires an authenticated session.
	if !s.authed {
		return fmt.Errorf("%s received before login", msg.Name)
	}
	switch msg.Name {
	case "C2S_SetOptions":
		return h.handleSetOptions(ctx, s, values)
	case "C2S_WaitingUserCount":
		return h.handleWaitingUserCount(s)
	case "C2S_PlayerList":
		return h.handlePlayerList(ctx, s)
	case "C2S_CreatePlayer":
		return h.handleCreatePlayer(ctx, s, values)
	case "C2S_Logout":
		return h.handleLogout(s)
	default:
		h.Logger.Info("unhandled message", "message", msg.Name)
		return nil
	}
}

// handleVersionCheck validates the echoed handshake identifiers and replies with
// the server's version/time.
func (h *Handler) handleVersionCheck(s *session, values protocol.Values) error {
	socketUID, _ := asInt64(values["m_socketUID"])
	connectionKey, _ := asInt64(values["m_connectionKey"])
	if socketUID != s.conn.SocketUID || connectionKey != s.conn.ConnectionKey {
		return fmt.Errorf("version check: handshake identifiers do not match")
	}
	clientVer, _ := asInt64(values["m_protocolVer"])
	if h.ProtocolVer != 0 && int32(clientVer) != h.ProtocolVer {
		return fmt.Errorf("version check: client %d != server %d", clientVer, h.ProtocolVer)
	}

	reply, err := protocol.ZeroValues("S2C_VersionCheck")
	if err != nil {
		return err
	}
	now := time.Now()
	reply["m_protocolVer"] = clientVer
	reply["m_serverID"] = int64(h.WorldID)
	reply["m_utcTimeOffset"] = int64(0)
	reply["m_serverTime"] = now.UnixMilli()
	reply["m_serverTick"] = now.UnixNano()
	return s.conn.Send("S2C_VersionCheck", reply)
}

// handleLogin validates the sessionKey against rom-api and, on success, replies
// with the account binding. Credentials are never checked here.
func (h *Handler) handleLogin(ctx context.Context, s *session, values protocol.Values) error {
	accountCode, _ := asString(values["m_accountCode"])
	sessionKeyRaw, _ := asInt64(values["m_sessionKey"])
	sessionKey := int32(sessionKeyRaw)

	session, err := h.Auth.Validate(ctx, sessionKey, accountCode)
	if err != nil {
		h.Logger.Warn("login rejected", "accountCode", accountCode, "sessionKey", sessionKey, "error", err)
		return h.sendLoginResult(s, accountCode, sessionKey, resultAuthFailed)
	}
	if session.WorldID != h.WorldID {
		h.Logger.Warn("login world mismatch", "session", session.WorldID, "server", h.WorldID)
		return h.sendLoginResult(s, accountCode, sessionKey, resultAuthFailed)
	}

	// Create/refresh the local account projection before any character work.
	if err := h.Store.UpsertAccount(ctx, session.AccountID, session.UserCode); err != nil {
		h.Logger.Error("upsert account failed", "accountId", session.AccountID, "error", err)
		return h.sendLoginResult(s, accountCode, sessionKey, resultAuthFailed)
	}

	s.accountID = session.AccountID
	s.accountCode = session.UserCode
	s.authed = true
	h.Logger.Info("login accepted", "accountId", s.accountID, "accountCode", s.accountCode, "sessionKey", sessionKey)
	return h.sendLoginResult(s, accountCode, sessionKey, resultOK)
}

// login result codes (m_result on S2C_SessionAuthLogin); 0 is success.
const (
	resultOK         = 0
	resultAuthFailed = 1
)

func (h *Handler) sendLoginResult(s *session, accountCode string, sessionKey int32, result int) error {
	reply, err := protocol.ZeroValues("S2C_SessionAuthLogin")
	if err != nil {
		return err
	}
	reply["m_result"] = int64(result)
	reply["m_accountCode"] = accountCode
	reply["m_sessionKey"] = int64(sessionKey)
	return s.conn.Send("S2C_SessionAuthLogin", reply)
}

func asInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case uint64:
		return int64(n), true
	default:
		return 0, false
	}
}

func asUint64(v any) (uint64, bool) {
	switch n := v.(type) {
	case uint64:
		return n, true
	case int64:
		return uint64(n), true
	default:
		return 0, false
	}
}

func asString(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}
