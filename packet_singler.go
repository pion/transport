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
	if len(ps) == 0 {
		return 0, nil
	}

	p := &ps[0]

	p.Size, err = psr.WriteTo(p.Buffer, p.Addr)
	if err != nil {
		return 0, err
	}

	return 1, nil
}
