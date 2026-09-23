package game

import (
	"testing"

	"github.com/octrys/rom-server-poc/internal/persist"
	"github.com/octrys/rom-server-poc/internal/protocol"
)

// TestEnterWorldCompleteRoundTrip fills a MyPlayerInfo from a character and
// verifies S2C_EnterWorldComplete encodes and decodes cleanly, guarding the
// nested struct/pose wire types in fillMyPlayerInfo.
func TestEnterWorldCompleteRoundTrip(t *testing.T) {
	c := &persist.Character{
		ID:        7,
		Name:      "Tanakku",
		ClassType: 1,
		HeadType:  11,
		Level:     1,
		MapID:     3,
		X:         70.0,
		Y:         50.25,
		Z:         38.25,
		Dir:       10.5,
	}
	h := &Handler{WorldID: 1}
	const oid = int64(1)*oidWorldMultiplier + 7
	const warpToken = int64(4787280687647936126)

	reply, err := protocol.ZeroValues("S2C_EnterWorldComplete")
	if err != nil {
		t.Fatalf("zero values: %v", err)
	}
	reply["m_result"] = int64(enterResultOK)
	reply["m_warpToken"] = warpToken
	info, ok := reply["m_myPlayerInfo"].(protocol.Values)
	if !ok {
		t.Fatalf("m_myPlayerInfo is %T, want protocol.Values", reply["m_myPlayerInfo"])
	}
	h.fillMyPlayerInfo(info, c, oid)

	body, err := protocol.Encode("S2C_EnterWorldComplete", reply)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	msg, out, tail, err := protocol.Decode(body)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if msg == nil || msg.Name != "S2C_EnterWorldComplete" {
		t.Fatalf("decoded wrong message: %v", msg)
	}
	if len(tail) != 0 {
		t.Fatalf("unexpected tail: % x", tail)
	}
	if out["m_warpToken"] != warpToken {
		t.Errorf("m_warpToken = %v, want %d", out["m_warpToken"], warpToken)
	}

	got, ok := out["m_myPlayerInfo"].(protocol.Values)
	if !ok {
		t.Fatalf("decoded m_myPlayerInfo is %T, want protocol.Values", out["m_myPlayerInfo"])
	}
	if got["m_OID"] != oid {
		t.Errorf("m_OID = %v, want %d", got["m_OID"], oid)
	}
	if got["m_name"] != "Tanakku" {
		t.Errorf("m_name = %v, want Tanakku", got["m_name"])
	}
	if got["m_mapIndex"] != int64(3) {
		t.Errorf("m_mapIndex = %v, want 3", got["m_mapIndex"])
	}
	pos, ok := got["m_pos"].(protocol.Values)
	if !ok {
		t.Fatalf("m_pos is %T, want protocol.Values", got["m_pos"])
	}
	if pos["m_x"] != float64(float32(70.0)) {
		t.Errorf("m_pos.m_x = %v, want 70.0", pos["m_x"])
	}
}
