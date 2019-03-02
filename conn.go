package transport

import (
	"net"
)

// Conn is an extension of net.Conn with batched support
type Conn interface {
	net.Conn

	// ReadBatch takes a slice of Messages, reading them in bulk if supported.
	// Returns the number of messages read, which may be less than the full slice.
	// Each message will have Buffer and Size populated.
	ReadBatch(ms []Message) (n int, err error)

	// WriteBatch takes a slice of Messages, writing them in bulk if supported.
	// Returns the number of messages written, which may be less than the full slice.
	// Each message will have Size populated with the number of bytes written.
	WriteBatch(ms []Message) (n int, err error)
}

// NewConn creates a Conn given a net.Conn
// This is a wrapper, added stubs for batched support when needed.
func NewConn(conn net.Conn) (c Conn) {
	switch ct := conn.(type) {
	case Conn:
		// Don't wrap if it already implements the interface.
		return ct
	default:
		// Otherwise use a wrapper that translates the batch calls into single reads/writes
		return newMessageSingler(conn)
	}
}
