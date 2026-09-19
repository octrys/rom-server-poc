// Package transport implements the ROM game-socket wire layer: length-prefixed
// framing, the Rabbit stream cipher, AES key derivation, and the plaintext
// handshake.
package transport

import (
	"encoding/binary"
	"math/bits"
)

// rabbitA are the RFC 4503 counter-increment constants.
var rabbitA = [8]uint32{
	0x4D34D34D, 0xD34D34D3, 0x34D34D34, 0x4D34D34D,
	0xD34D34D3, 0x34D34D34, 0x4D34D34D, 0xD34D34D3,
}

// Rabbit is the eSTREAM / RFC 4503 stream cipher used for the ROM transport:
// a 128-bit key and optional 64-bit IV, producing a keystream XORed with each
// frame body. One instance is created per direction at handshake.
type Rabbit struct {
	x   [8]uint32
	c   [8]uint32
	b   uint32
	buf []byte // spare keystream bytes not yet consumed
}

// NewRabbit constructs the cipher from a 16-byte key and an optional 8-byte IV
// (pass nil for no IV setup).
func NewRabbit(key []byte, iv []byte) *Rabbit {
	var k [8]uint32
	for i := 0; i < 8; i++ {
		k[i] = uint32(key[2*i]) | uint32(key[2*i+1])<<8 // 8x 16-bit LE
	}
	r := &Rabbit{}
	for j := 0; j < 8; j++ {
		if j%2 == 0 {
			r.x[j] = k[(j+1)%8]<<16 | k[j]
			r.c[j] = k[(j+4)%8]<<16 | k[(j+5)%8]
		} else {
			r.x[j] = k[(j+5)%8]<<16 | k[(j+4)%8]
			r.c[j] = k[j]<<16 | k[(j+1)%8]
		}
	}
	for i := 0; i < 4; i++ {
		r.next()
	}
	for j := 0; j < 8; j++ {
		r.c[j] ^= r.x[(j+4)%8]
	}
	if iv != nil {
		i0 := binary.LittleEndian.Uint32(iv[0:])
		i1 := binary.LittleEndian.Uint32(iv[4:])
		hi := (i1>>16)<<16 | (i0 >> 16)
		lo := (i1&0xFFFF)<<16 | (i0 & 0xFFFF)
		r.c[0] ^= i0
		r.c[1] ^= hi
		r.c[2] ^= i1
		r.c[3] ^= lo
		r.c[4] ^= i0
		r.c[5] ^= hi
		r.c[6] ^= i1
		r.c[7] ^= lo
		for i := 0; i < 4; i++ {
			r.next()
		}
	}
	return r
}

func rabbitG(u, v uint32) uint32 {
	s := u + v
	sq := uint64(s) * uint64(s)
	return uint32(sq) ^ uint32(sq>>32)
}

func (r *Rabbit) next() {
	b := r.b
	for j := 0; j < 8; j++ {
		t := r.c[j] + rabbitA[j] + b
		if t < r.c[j] {
			b = 1
		} else {
			b = 0
		}
		r.c[j] = t
	}
	var g [8]uint32
	for j := 0; j < 8; j++ {
		g[j] = rabbitG(r.x[j], r.c[j])
	}
	r.x[0] = g[0] + bits.RotateLeft32(g[7], 16) + bits.RotateLeft32(g[6], 16)
	r.x[1] = g[1] + bits.RotateLeft32(g[0], 8) + g[7]
	r.x[2] = g[2] + bits.RotateLeft32(g[1], 16) + bits.RotateLeft32(g[0], 16)
	r.x[3] = g[3] + bits.RotateLeft32(g[2], 8) + g[1]
	r.x[4] = g[4] + bits.RotateLeft32(g[3], 16) + bits.RotateLeft32(g[2], 16)
	r.x[5] = g[5] + bits.RotateLeft32(g[4], 8) + g[3]
	r.x[6] = g[6] + bits.RotateLeft32(g[5], 16) + bits.RotateLeft32(g[4], 16)
	r.x[7] = g[7] + bits.RotateLeft32(g[6], 8) + g[5]
	r.b = b
}

// Keystream returns the next n keystream bytes.
func (r *Rabbit) Keystream(n int) []byte {
	for len(r.buf) < n {
		r.next()
		var block [16]byte
		binary.LittleEndian.PutUint32(block[0:], r.x[0]^(r.x[5]>>16)^(r.x[3]<<16))
		binary.LittleEndian.PutUint32(block[4:], r.x[2]^(r.x[7]>>16)^(r.x[5]<<16))
		binary.LittleEndian.PutUint32(block[8:], r.x[4]^(r.x[1]>>16)^(r.x[7]<<16))
		binary.LittleEndian.PutUint32(block[12:], r.x[6]^(r.x[3]>>16)^(r.x[1]<<16))
		r.buf = append(r.buf, block[:]...)
	}
	out := make([]byte, n)
	copy(out, r.buf[:n])
	r.buf = r.buf[n:]
	return out
}
