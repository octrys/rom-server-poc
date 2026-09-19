package transport

import (
	"bufio"
	"crypto/aes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"net"

	"github.com/octrys/rom-server-poc/internal/protocol"
)

// StaticKey is the AES-128-ECB key the client uses to derive the per-session
// body key. It is rotated on client updates (this is the post-2026-09 value).
var StaticKey = []byte("Sxf89J&1*,0Axm>t")

// Conn is a keyed game session over a raw TCP socket: it frames, encrypts, and
// decrypts messages after the plaintext handshake. Not safe for concurrent use
// by multiple writers; serialize sends through the owning goroutine.
type Conn struct {
	raw net.Conn
	rx  *bufio.Reader

	c2s *Rabbit // client-send keystream: decrypts inbound frames
	s2c *Rabbit // server-send keystream: encrypts outbound frames

	// SocketUID and ConnectionKey are the handshake identifiers the client
	// echoes back in C2S_VersionCheck, for validation.
	SocketUID     int64
	ConnectionKey int64
}

// Handshake takes an accepted TCP connection, generates the session keys, sends
// the plaintext S2C_CheckConnection frame, and returns a keyed Conn. The client
// derives the same body key from the encryption-key halves we send.
func Handshake(raw net.Conn) (*Conn, error) {
	socketUID, err := randInt63()
	if err != nil {
		return nil, err
	}
	connectionKey, err := randInt63()
	if err != nil {
		return nil, err
	}
	clientSendIV, err := randInt63()
	if err != nil {
		return nil, err
	}
	serverSendIV, err := randInt63()
	if err != nil {
		return nil, err
	}

	rawKey := make([]byte, 16)
	if _, err := rand.Read(rawKey); err != nil {
		return nil, fmt.Errorf("transport: generate key: %w", err)
	}
	encKeyLow := int64(binary.LittleEndian.Uint64(rawKey[0:8]))
	encKeyHigh := int64(binary.LittleEndian.Uint64(rawKey[8:16]))

	derived, err := deriveKey(rawKey)
	if err != nil {
		return nil, err
	}

	c := &Conn{
		raw:           raw,
		rx:            bufio.NewReader(raw),
		c2s:           NewRabbit(derived, int64LE(clientSendIV)),
		s2c:           NewRabbit(derived, int64LE(serverSendIV)),
		SocketUID:     socketUID,
		ConnectionKey: connectionKey,
	}

	body, err := protocol.Encode("S2C_CheckConnection", protocol.Values{
		"m_socketUID":         socketUID,
		"m_connectionKey":     connectionKey,
		"m_clientSendIV":      clientSendIV,
		"m_serverSendIV":      serverSendIV,
		"m_encryptionKeyLow":  encKeyLow,
		"m_encryptionKeyHigh": encKeyHigh,
	})
	if err != nil {
		return nil, fmt.Errorf("transport: encode handshake: %w", err)
	}
	if err := c.writeFrame(body); err != nil { // plaintext: no keystream consumed
		return nil, err
	}
	return c, nil
}

// Send encodes a message and writes it as an encrypted frame.
func (c *Conn) Send(name string, values protocol.Values) error {
	plain, err := protocol.Encode(name, values)
	if err != nil {
		return err
	}
	return c.writeFrame(crypt(c.s2c, plain))
}

// Recv reads one frame, decrypts it, and decodes it. It returns io.EOF when the
// peer closes the connection.
func (c *Conn) Recv() (*protocol.Message, protocol.Values, []byte, error) {
	body, err := c.readFrame()
	if err != nil {
		return nil, nil, nil, err
	}
	return protocol.Decode(crypt(c.c2s, body))
}

// Close closes the underlying socket.
func (c *Conn) Close() error { return c.raw.Close() }

// RemoteAddr reports the peer address, for logging.
func (c *Conn) RemoteAddr() net.Addr { return c.raw.RemoteAddr() }

// readFrame reads one length-prefixed frame and returns its (still-encrypted)
// body. The u16 length is plaintext and includes its own 2 bytes.
func (c *Conn) readFrame() ([]byte, error) {
	var header [2]byte
	if _, err := io.ReadFull(c.rx, header[:]); err != nil {
		return nil, err
	}
	length := int(binary.LittleEndian.Uint16(header[:]))
	if length < 2 {
		return nil, fmt.Errorf("transport: bogus frame length %d", length)
	}
	body := make([]byte, length-2)
	if _, err := io.ReadFull(c.rx, body); err != nil {
		return nil, err
	}
	return body, nil
}

func (c *Conn) writeFrame(body []byte) error {
	header := make([]byte, 2)
	binary.LittleEndian.PutUint16(header, uint16(len(body)+2))
	if _, err := c.raw.Write(append(header, body...)); err != nil {
		return fmt.Errorf("transport: write frame: %w", err)
	}
	return nil
}

// crypt XORs a body with a block-aligned slice of the direction's keystream:
// each frame consumes ceil(len/16)*16 keystream bytes so the next frame starts
// on a 16-byte boundary.
func crypt(r *Rabbit, body []byte) []byte {
	aligned := ((len(body) + 15) / 16) * 16
	ks := r.Keystream(aligned)
	out := make([]byte, len(body))
	for i := range body {
		out[i] = body[i] ^ ks[i]
	}
	return out
}

// deriveKey computes the body key: AES-128-ECB-decrypt(StaticKey, rawKey16).
func deriveKey(rawKey []byte) ([]byte, error) {
	block, err := aes.NewCipher(StaticKey)
	if err != nil {
		return nil, fmt.Errorf("transport: static key: %w", err)
	}
	derived := make([]byte, 16)
	block.Decrypt(derived, rawKey)
	return derived, nil
}

// int64LE returns the little-endian 8-byte encoding of v, used as a Rabbit IV.
func int64LE(v int64) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(v))
	return b
}

// randInt63 returns a random non-negative int64 for a handshake identifier.
func randInt63() (int64, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, fmt.Errorf("transport: random: %w", err)
	}
	return int64(binary.LittleEndian.Uint64(b[:]) >> 1), nil
}
