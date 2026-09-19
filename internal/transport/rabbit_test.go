package transport

import "testing"

// Golden keystreams for the Rabbit cipher (RFC 4503), verified against the live
// production server, guarding against regressions in this implementation.
func TestRabbitGoldenVectors(t *testing.T) {
	key16 := make([]byte, 16)
	for i := range key16 {
		key16[i] = byte(i)
	}

	cases := []struct {
		name string
		key  []byte
		iv   []byte
		n    int
		want string
	}{
		{
			name: "sequential key, no IV",
			key:  key16,
			n:    64,
			want: "08404f232bf002175aaf97e92e6e5fe52e6f26497e5e027f931f48b08c51c49d" +
				"7004d864cc8f2451e03c4cc8c7c94f5496a49c4b694144dd4c33bfe211d8256f",
		},
		{
			name: "sequential key, with IV",
			key:  key16,
			iv:   []byte{0xAA, 0xBB, 0xCC, 0xDD, 0x01, 0x02, 0x03, 0x04},
			n:    64,
			want: "49082cd10ead88cc22772034bc972ac0dd5f39140d05f6d21769fd78381dbc40" +
				"fca011057e192ba6686efcec2d20b961a41dd1a703bb7504f32e871de2ffa23e",
		},
		{
			name: "derived-style key, no IV",
			key:  mustHex("4d8a1953efda4b63ed1edc0e0d4d2c42"),
			n:    32,
			want: "970914f2655575c6c674dbb14cf24f95c297b3c44275e42440f04e3ffc823b13",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := toHex(NewRabbit(tc.key, tc.iv).Keystream(tc.n))
			if got != tc.want {
				t.Fatalf("keystream mismatch\n got: %s\nwant: %s", got, tc.want)
			}
		})
	}
}

func toHex(b []byte) string {
	const hexChars = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, c := range b {
		out[2*i] = hexChars[c>>4]
		out[2*i+1] = hexChars[c&0x0F]
	}
	return string(out)
}

func mustHex(s string) []byte {
	out := make([]byte, len(s)/2)
	for i := range out {
		out[i] = fromHexNibble(s[2*i])<<4 | fromHexNibble(s[2*i+1])
	}
	return out
}

func fromHexNibble(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	default:
		return 0
	}
}
