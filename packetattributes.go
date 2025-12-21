// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package transport

const MaxAttributesLen = 1024

type PacketAttributes struct {
	Buffer      []byte
	BytesCopied int
}

func (p *PacketAttributes) Reset() {
	p.BytesCopied = 0
}

func NewPacketAttributesWithLen(length int) *PacketAttributes {
	buff := make([]byte, length)

	return &PacketAttributes{
		Buffer:      buff,
		BytesCopied: 0,
	}
}

func (p *PacketAttributes) Clone() *PacketAttributes {
	b := make([]byte, p.BytesCopied)
	copy(b, p.Buffer)
	return &PacketAttributes{
		Buffer:      b,
		BytesCopied: p.BytesCopied,
	}
}

// Returns the read buffer. Just like when calling a read on a socket we have n, err := conn.Read(buf)
// and the read bytes are in buf[:n], we should use this method after calling the ReadWithAttributes method.
func (p *PacketAttributes) GetReadPacketAttributes() *PacketAttributes {
	return &PacketAttributes{
		Buffer:      p.Buffer[:p.BytesCopied],
		BytesCopied: p.BytesCopied,
	}
}
