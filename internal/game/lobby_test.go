package game

import (
	"testing"
	"time"

	"github.com/octrys/rom-server-poc/internal/persist"
	"github.com/octrys/rom-server-poc/internal/protocol"
)

// TestPlayerListRoundTrip builds an S2C_PlayerList from a character and verifies
// it encodes and decodes cleanly — guarding the wire types in toLobbyPlayerInfo.
func TestPlayerListRoundTrip(t *testing.T) {
	c := persist.Character{
		ID:        1,
		SlotIndex: 1,
		Name:      "Archer44444",
		ClassType: 2,
		HeadType:  21,
		Level:     1,
		MapID:     501,
		CreatedAt: time.Unix(1789801837, 0),
	}

	reply, err := protocol.ZeroValues("S2C_PlayerList")
	if err != nil {
		t.Fatalf("zero values: %v", err)
	}
	reply["m_maxSlotCount"] = uint64(maxCharacterSlots)
	reply["m_playerList"] = []any{toLobbyPlayerInfo(c)}

	body, err := protocol.Encode("S2C_PlayerList", reply)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	msg, out, tail, err := protocol.Decode(body)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if msg == nil || msg.Name != "S2C_PlayerList" {
		t.Fatalf("decoded wrong message: %v", msg)
	}
	if len(tail) != 0 {
		t.Fatalf("unexpected tail: % x", tail)
	}
	if got := out["m_maxSlotCount"]; got != uint64(maxCharacterSlots) {
		t.Fatalf("m_maxSlotCount = %v, want %d", got, maxCharacterSlots)
	}

	players, ok := out["m_playerList"].([]any)
	if !ok || len(players) != 1 {
		t.Fatalf("m_playerList = %#v, want one element", out["m_playerList"])
	}
	player, ok := players[0].(protocol.Values)
	if !ok {
		t.Fatalf("player element is %T, want protocol.Values", players[0])
	}
	if player["m_name"] != "Archer44444" {
		t.Errorf("m_name = %v, want Archer44444", player["m_name"])
	}
	if player["m_classType"] != uint64(2) {
		t.Errorf("m_classType = %v, want 2", player["m_classType"])
	}
	if player["m_mapIndex"] != int64(501) {
		t.Errorf("m_mapIndex = %v, want 501", player["m_mapIndex"])
	}
	if player["m_createdTime"] != int64(1789801837) {
		t.Errorf("m_createdTime = %v, want 1789801837", player["m_createdTime"])
	}
}

func TestFreeSlot(t *testing.T) {
	cases := []struct {
		name     string
		slots    []int32
		wantSlot int32
		wantOK   bool
	}{
		{"empty", nil, 0, true},
		{"slot 0 taken", []int32{0}, 1, true},
		{"gap at 1", []int32{0, 2}, 1, true},
		{"full", []int32{0, 1, 2}, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			characters := make([]persist.Character, len(tc.slots))
			for i, slot := range tc.slots {
				characters[i] = persist.Character{SlotIndex: slot}
			}
			slot, ok := freeSlot(characters)
			if ok != tc.wantOK || (ok && slot != tc.wantSlot) {
				t.Fatalf("freeSlot(%v) = (%d, %v), want (%d, %v)", tc.slots, slot, ok, tc.wantSlot, tc.wantOK)
			}
		})
	}
}
