package game

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/octrys/rom-server-poc/internal/persist"
	"github.com/octrys/rom-server-poc/internal/protocol"
	"github.com/octrys/rom-server-poc/internal/world"
)

// enter-world result codes (from the client's error enum). 0 is success.
const (
	enterResultOK      = 0     // ERR_SUCCESS
	enterResultNoChar  = 50101 // ERR_DB_NOT_EXIST_CHAR
	mapLayerIndex      = 1     // single-layer maps for now
	oidWorldMultiplier = 1_000_000_000
)

// handleEnterWorld selects the character in the requested slot, admits it to the
// world, and drives the enter sequence: S2C_EnterWorld then S2C_EnterWorldComplete
// (carrying the full player info and the warp token the client must echo back on
// C2S_MapEnter).
func (h *Handler) handleEnterWorld(ctx context.Context, s *session, values protocol.Values) error {
	slot, _ := asUint64(values["m_playerSlotIndex"])

	characters, err := h.Store.ListCharacters(ctx, s.accountID)
	if err != nil {
		return fmt.Errorf("enter world: %w", err)
	}
	var chosen *persist.Character
	for i := range characters {
		if uint64(characters[i].SlotIndex) == slot {
			chosen = &characters[i]
			break
		}
	}
	if chosen == nil {
		h.Logger.Warn("enter world: no character in slot", "accountId", s.accountID, "slot", slot)
		return h.sendEnterWorldResult(s, enterResultNoChar, int32(slot))
	}

	token, err := newWarpToken()
	if err != nil {
		return fmt.Errorf("enter world: %w", err)
	}
	s.character = chosen
	s.playerOID = int64(h.WorldID)*oidWorldMultiplier + chosen.ID
	s.warpToken = token
	s.inWorld = true

	h.World.Join(ctx, &world.Player{
		OID:         s.playerOID,
		CharacterID: chosen.ID,
		AccountID:   s.accountID,
		Name:        chosen.Name,
		MapID:       chosen.MapID,
		X:           chosen.X,
		Y:           chosen.Y,
		Z:           chosen.Z,
		Dir:         chosen.Dir,
		Conn:        s.conn,
	})
	h.Logger.Info("enter world", "accountId", s.accountID, "character", chosen.Name,
		"oid", s.playerOID, "map", chosen.MapID)

	if err := h.sendEnterWorldResult(s, enterResultOK, chosen.SlotIndex); err != nil {
		return err
	}
	return h.sendEnterWorldComplete(s)
}

func (h *Handler) sendEnterWorldResult(s *session, result int, slot int32) error {
	reply, err := protocol.ZeroValues("S2C_EnterWorld")
	if err != nil {
		return err
	}
	reply["m_result"] = int64(result)
	reply["m_playerSlotIndex"] = uint64(slot)
	return s.conn.Send("S2C_EnterWorld", reply)
}

func (h *Handler) sendEnterWorldComplete(s *session) error {
	reply, err := protocol.ZeroValues("S2C_EnterWorldComplete")
	if err != nil {
		return err
	}
	reply["m_result"] = int64(enterResultOK)
	reply["m_warpToken"] = s.warpToken
	if info, ok := reply["m_myPlayerInfo"].(protocol.Values); ok {
		h.fillMyPlayerInfo(info, s.character, s.playerOID)
	}
	return s.conn.Send("S2C_EnterWorldComplete", reply)
}

// fillMyPlayerInfo overrides the identity and pose fields on a zeroed MyPlayerInfo
// struct. Fields we do not yet model (stats, equipment, guild) stay zero.
func (h *Handler) fillMyPlayerInfo(info protocol.Values, c *persist.Character, oid int64) {
	info["m_OID"] = oid
	info["m_worldID"] = int64(h.WorldID)
	info["m_name"] = c.Name
	info["m_class"] = uint64(c.ClassType)
	info["m_subClass"] = uint64(c.SubClassType)
	info["m_headIndex"] = int64(c.HeadType)
	info["m_level"] = int64(c.Level)
	info["m_exp"] = c.Exp
	info["m_mapIndex"] = int64(c.MapID)
	info["m_dir"] = float64(c.Dir)
	info["m_useWorldAuction"] = true
	if pos, ok := info["m_pos"].(protocol.Values); ok {
		pos["m_x"] = float64(c.X)
		pos["m_y"] = float64(c.Y)
		pos["m_z"] = float64(c.Z)
	}
}

// handleMapEnter validates the echoed warp token and confirms the map load,
// replying with the spawn pose the client renders at.
func (h *Handler) handleMapEnter(ctx context.Context, s *session, values protocol.Values) error {
	if s.character == nil {
		return fmt.Errorf("map enter before enter world")
	}
	token, _ := asInt64(values["m_warpToken"])
	if token != s.warpToken {
		return fmt.Errorf("map enter: warp token mismatch")
	}
	mapIndex, _ := asInt64(values["m_mapIndex"])
	if int32(mapIndex) != s.character.MapID {
		h.Logger.Warn("map enter: map mismatch", "requested", mapIndex, "character", s.character.MapID)
	}

	reply, err := protocol.ZeroValues("S2C_MapEnter")
	if err != nil {
		return err
	}
	now := time.Now()
	reply["m_result"] = int64(enterResultOK)
	reply["m_mapIndex"] = int64(s.character.MapID)
	reply["m_layerIndex"] = int64(mapLayerIndex)
	reply["m_dir"] = float64(s.character.Dir)
	reply["m_serverID"] = int64(h.WorldID)
	reply["m_serverTick"] = now.UnixNano()
	if pos, ok := reply["m_startPos"].(protocol.Values); ok {
		pos["m_x"] = float64(s.character.X)
		pos["m_y"] = float64(s.character.Y)
		pos["m_z"] = float64(s.character.Z)
	}
	return s.conn.Send("S2C_MapEnter", reply)
}

// handleMapEnterPreComplete acknowledges the client's mid-load checkpoint.
func (h *Handler) handleMapEnterPreComplete(s *session) error {
	reply, err := protocol.ZeroValues("S2C_MapEnterPreComplete")
	if err != nil {
		return err
	}
	return s.conn.Send("S2C_MapEnterPreComplete", reply)
}

// handleMapEnterComplete finalises the map load; the client is now in-world.
func (h *Handler) handleMapEnterComplete(s *session) error {
	if s.character == nil {
		return fmt.Errorf("map enter complete before enter world")
	}
	reply, err := protocol.ZeroValues("S2C_MapEnterComplete")
	if err != nil {
		return err
	}
	reply["m_result"] = int64(enterResultOK)
	reply["m_playerOID"] = s.playerOID
	return s.conn.Send("S2C_MapEnterComplete", reply)
}

// handlePlayerLoadFirstCall answers the client's post-load handshake. The bulk
// data lists (equipment, skills, items) are sent on demand as those handlers land.
func (h *Handler) handlePlayerLoadFirstCall(s *session) error {
	reply, err := protocol.ZeroValues("S2C_PlayerLoadFirstCall")
	if err != nil {
		return err
	}
	reply["m_result"] = int64(enterResultOK)
	return s.conn.Send("S2C_PlayerLoadFirstCall", reply)
}

// newWarpToken returns a non-zero random token bound to a single enter-world
// exchange, so a client cannot reuse or forge one on C2S_MapEnter.
func newWarpToken() (int64, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return 0, fmt.Errorf("warp token: %w", err)
	}
	token := int64(binary.LittleEndian.Uint64(buf[:]))
	if token == 0 {
		token = 1
	}
	return token, nil
}
