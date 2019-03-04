package transport

import (
	"net"
)

type packetSingler struct {
	net.PacketConn
}

func newPacketSingler(conn net.PacketConn) (ps *packetSingler) {
	return &packetSingler{conn}
}

func (psr *packetSingler) ReadBatch(ps []Packet) (n int, err error) {
	if len(ps) == 0 {
		return 0, nil
	}

	// We can only read a single packet with blocking IO.
	// It may be worth implementing non-blocking IO to slightly improve performance.

	// The pointer is required to modify the slice value.
	p := &ps[0]

	size, addr, err := psr.ReadFrom(p.Buffer)
	if err != nil {
		return 0, err
	}

	p.Addr = addr
	p.Size = size

	return 1, nil
}

func (psr *packetSingler) WriteBatch(ps []Packet) (n int, err error) {
	// We could just write a single packet, but loop for better performance.
	// This will avoid function overhead and having the caller potentially reconstruct the Packet slice.

	for i := range ps {
		// The pointer is required to modify the slice value.
		p := &ps[i]

		p.Size, err = psr.WriteTo(p.Buffer, p.Addr)
		if err != nil {
			// Return the number of successful packets on error.
			return i, err
		}
	}

	return len(ps), nil
}
