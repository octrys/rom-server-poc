package game

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/octrys/rom-server-poc/internal/persist"
	"github.com/octrys/rom-server-poc/internal/protocol"
)

// maxCharacterSlots is the per-account character cap (S2C_PlayerList.m_maxSlotCount).
const maxCharacterSlots = 3

// character name bounds; the exact client limit is unconfirmed, so keep it lenient.
const (
	minNameLen = 1
	maxNameLen = 16
)

// S2C_CreatePlayer.m_result codes. 0 is success; the client's specific failure
// codes are not yet reversed, so failures use a single generic non-zero value.
const (
	createResultOK    = 0
	createResultError = 1
)

// startMaps maps a class to its newbie start map. Only class 2 -> 501 is
// confirmed from captures; other classes fall back to defaultStartMap until we
// have data for them.
var startMaps = map[int32]int32{
	2: 501,
}

const defaultStartMap = 501

func startMapForClass(classType int32) int32 {
	if m, ok := startMaps[classType]; ok {
		return m
	}
	return defaultStartMap
}

// handleSetOptions persists the account's client option bitmask. It has no reply
// and is non-critical, so a storage failure is logged but does not drop the client.
func (h *Handler) handleSetOptions(ctx context.Context, s *session, values protocol.Values) error {
	optionValues, _ := asUint64(values["m_optionValues"])
	if err := h.Store.SaveOptions(ctx, s.accountID, uint32(optionValues)); err != nil {
		h.Logger.Warn("save options failed", "accountId", s.accountID, "error", err)
	}
	return nil
}

// handleWaitingUserCount answers the login-queue poll with "no queue".
func (h *Handler) handleWaitingUserCount(s *session) error {
	reply, err := protocol.ZeroValues("S2C_WaitingUserCount")
	if err != nil {
		return err
	}
	reply["m_nowServerTime"] = time.Now().Unix()
	return s.conn.Send("S2C_WaitingUserCount", reply)
}

// handlePlayerList sends the account's character-select list.
func (h *Handler) handlePlayerList(ctx context.Context, s *session) error {
	characters, err := h.Store.ListCharacters(ctx, s.accountID)
	if err != nil {
		return fmt.Errorf("player list: %w", err)
	}
	players := make([]any, 0, len(characters))
	for _, c := range characters {
		players = append(players, toLobbyPlayerInfo(c))
	}

	reply, err := protocol.ZeroValues("S2C_PlayerList")
	if err != nil {
		return err
	}
	reply["m_maxSlotCount"] = uint64(maxCharacterSlots)
	reply["m_playerList"] = players
	reply["m_enableStoarge"] = false
	return s.conn.Send("S2C_PlayerList", reply)
}

// handleCreatePlayer validates the request, assigns a slot and start map, persists
// the new character, and replies with it.
func (h *Handler) handleCreatePlayer(ctx context.Context, s *session, values protocol.Values) error {
	name, _ := asString(values["m_name"])
	name = strings.TrimSpace(name)
	classType, _ := asUint64(values["m_classType"])
	headType, _ := asUint64(values["m_headType"])

	if length := utf8.RuneCountInString(name); length < minNameLen || length > maxNameLen {
		h.Logger.Warn("create rejected: bad name", "name", name)
		return h.sendCreateResult(s, createResultError, nil)
	}

	characters, err := h.Store.ListCharacters(ctx, s.accountID)
	if err != nil {
		return fmt.Errorf("create player: %w", err)
	}
	slot, ok := freeSlot(characters)
	if !ok {
		h.Logger.Warn("create rejected: slots full", "accountId", s.accountID)
		return h.sendCreateResult(s, createResultError, nil)
	}

	created, err := h.Store.CreateCharacter(ctx, persist.Character{
		AccountID: s.accountID,
		SlotIndex: slot,
		Name:      name,
		ClassType: int32(classType),
		HeadType:  int32(headType),
		Level:     1,
		MapID:     startMapForClass(int32(classType)),
	})
	if err != nil {
		if isCreateConflict(err) {
			h.Logger.Info("create rejected: conflict", "name", name, "error", err)
			return h.sendCreateResult(s, createResultError, nil)
		}
		return fmt.Errorf("create player: %w", err)
	}

	h.Logger.Info("character created", "accountId", s.accountID, "name", name, "slot", slot)
	lobby := toLobbyPlayerInfo(created)
	return h.sendCreateResult(s, createResultOK, lobby)
}

// handleLogout acknowledges a lobby logout.
func (h *Handler) handleLogout(s *session) error {
	reply, err := protocol.ZeroValues("S2C_Logout")
	if err != nil {
		return err
	}
	return s.conn.Send("S2C_Logout", reply)
}

func (h *Handler) sendCreateResult(s *session, result int, player protocol.Values) error {
	reply, err := protocol.ZeroValues("S2C_CreatePlayer")
	if err != nil {
		return err
	}
	reply["m_result"] = int64(result)
	if player != nil {
		reply["m_player"] = player
	}
	return s.conn.Send("S2C_CreatePlayer", reply)
}

// freeSlot returns the lowest unused slot index below the cap, or ok=false when full.
func freeSlot(characters []persist.Character) (int32, bool) {
	used := make(map[int32]bool, len(characters))
	for _, c := range characters {
		used[c.SlotIndex] = true
	}
	for slot := int32(0); slot < maxCharacterSlots; slot++ {
		if !used[slot] {
			return slot, true
		}
	}
	return 0, false
}

func isCreateConflict(err error) bool {
	return errors.Is(err, persist.ErrNameTaken) || errors.Is(err, persist.ErrSlotTaken)
}

// toLobbyPlayerInfo maps a persisted character to the wire LobbyPlayerInfo struct.
func toLobbyPlayerInfo(c persist.Character) protocol.Values {
	return protocol.Values{
		"m_slotIndex":          uint64(c.SlotIndex),
		"m_classType":          uint64(c.ClassType),
		"m_subClassType":       uint64(c.SubClassType),
		"m_name":               c.Name,
		"m_headType":           uint64(c.HeadType),
		"m_equipCostumeIndex":  int64(c.EquipCostumeIndex),
		"m_equipCostumeStepUp": uint64(c.EquipCostumeStepUp),
		"m_weaponItemIndex":    int64(c.WeaponItemIndex),
		"m_weaponEnchant":      uint64(c.WeaponEnchant),
		"m_level":              int64(c.Level),
		"m_exp":                c.Exp,
		"m_realPower":          int64(c.RealPower),
		"m_mapIndex":           int64(c.MapID),
		"m_latestLoginTime":    c.LatestLoginTime,
		"m_latestLogoutTime":   c.LatestLogoutTime,
		"m_deletedTime":        c.DeletedTime,
		"m_createdTime":        c.CreatedAt.Unix(),
		"m_guildName":          c.GuildName,
	}
}
