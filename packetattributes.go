// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package transport

const attrMaxLen = 1

type ECN uint8

type PacketAttributesBuffer interface {
	// for serializing
	Marshal() []byte

	// for de-serializing. The bytes will be copied into the returned buffer
	GetBuffer() []byte
}

type PacketAttributes struct {
	buffer [attrMaxLen]byte
}

func NewPacketAttributes() *PacketAttributes {
	p := &PacketAttributes{}
	p.Reset()

	return p
}

func (p *PacketAttributes) Reset() {
	p.WithECN(0)
}

func (p *PacketAttributes) GetECN() ECN {
	return ECN(p.buffer[0])
}

func (p *PacketAttributes) WithECN(e ECN) *PacketAttributes {
	p.buffer[0] = byte(e)

	return p
}

// Marshal returns the internal buffer as-is.
func (p *PacketAttributes) Marshal() []byte {
	return p.buffer[:]
}

func (p *PacketAttributes) GetBuffer() []byte {
	return p.buffer[:]
}

func (p *PacketAttributes) Clone() *PacketAttributes {
	clone := &PacketAttributes{}
	clone.buffer[0] = p.buffer[0]

	return clone
}
