package transport

import (
	"net"

	"golang.org/x/net/ipv4"
)

type packetBatcher struct {
	net.PacketConn

	ipv4 *ipv4.PacketConn
}

func newPacketBatcher(conn net.PacketConn) (pb *packetBatcher) {
	return &packetBatcher{
		PacketConn: conn,
		ipv4:       ipv4.NewPacketConn(conn),
	}
}

func (pb *packetBatcher) ReadBatch(ps []Packet) (n int, err error) {
	messages := make([]ipv4.Message, len(ps))

	for i, p := range ps {
		messages[i] = ipv4.Message{
			Addr:    p.Addr,
			Buffers: [][]byte{p.Buffer},
		}
	}

	n, err = pb.ipv4.ReadBatch(messages, 0)

	for i := 0; i < n; i++ {
		ps[i].Size = messages[i].N
	}

	return n, err
}

func (pb *packetBatcher) WriteBatch(ps []Packet) (n int, err error) {
	messages := make([]ipv4.Message, len(ps))

	for i, p := range ps {
		messages[i] = ipv4.Message{
			Addr:    p.Addr,
			Buffers: [][]byte{p.Buffer},
		}
	}

	n, err = pb.ipv4.WriteBatch(messages, 0)

	for i := 0; i < n; i++ {
		ps[i].Size = messages[i].N
	}

	return n, err
}
