package transport

import (
	"net"
)

// Packet is a message also containing an address.
type Packet struct {
	Addr net.Addr
	Message
}
