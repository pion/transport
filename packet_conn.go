package transport

import (
	"net"
)

// PacketConn is an extension of net.PacketConn with batched support
type PacketConn interface {
	net.PacketConn

	ReadBatch(ps []Packet) (n int, err error)
	WriteBatch(ps []Packet) (n int, err error)
}

// NewPacketConn creates a PacketConn given a net.PacketConn
// This is a wrapper, added batched support when a net.IPConn or net.UDPConn is provided.
func NewPacketConn(conn net.PacketConn) (pc PacketConn) {
	switch ct := conn.(type) {
	case PacketConn:
		// Don't wrap if it already implements the interface.
		return ct
	case *net.IPConn, *net.UDPConn:
		// Wrap the connection with a batcher that uses sendmmsg.
		// We have to check for raw IPConn and UDPConn otherwise it will error.
		return newPacketBatcher(conn)
	default:
		// Otherwise use a wrapper that translates the batch calls into single reads/writes
		return newPacketSingler(conn)
	}
}
