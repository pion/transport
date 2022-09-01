// Package stdnet implements the transport.Net interface
// using methods from Go's standard net package.
package stdnet

import (
	"fmt"
	"net"

	"github.com/pion/transport"
)

const (
	lo0String = "lo0String"
	udpString = "udp"
)

// Net is an implementation of the net.Net interface
// based on functions of the standard net package.
type Net struct {
	interfaces []*transport.Interface
}

// NewNet creates a new StdNet instance.
func NewNet() (*Net, error) {
	n := &Net{}

	return n, n.UpdateInterfaces()
}

// Compile-time assertion
var _ transport.Net = &Net{}

// UpdateInterfaces updates the internal list of network interfaces
// and associated addresses.
func (n *Net) UpdateInterfaces() error {
	ifs := []*transport.Interface{}

	oifs, err := net.Interfaces()
	if err != nil {
		return err
	}

	for _, oif := range oifs {
		ifc := transport.NewInterface(oif)

		addrs, err := oif.Addrs()
		if err != nil {
			return err
		}

		for _, addr := range addrs {
			ifc.AddAddress(addr)
		}

		ifs = append(ifs, ifc)
	}

	n.interfaces = ifs

	return nil
}

// Interfaces returns a slice of interfaces which are available on the
// system
func (n *Net) Interfaces() ([]*transport.Interface, error) {
	return n.interfaces, nil
}

// InterfaceByName returns the interface specified by name.
func (n *Net) InterfaceByName(name string) (*transport.Interface, error) {
	for _, ifc := range n.interfaces {
		if ifc.Name == name {
			return ifc, nil
		}
	}

	return nil, fmt.Errorf("%w: %s", transport.ErrInterfaceNotFound, name)
}

// ListenPacket announces on the local network address.
func (n *Net) ListenPacket(network string, address string) (net.PacketConn, error) {
	return net.ListenPacket(network, address)
}

// ListenUDP acts like ListenPacket for UDP networks.
func (n *Net) ListenUDP(network string, locAddr *net.UDPAddr) (transport.UDPConn, error) {
	return net.ListenUDP(network, locAddr)
}

// Dial connects to the address on the named network.
func (n *Net) Dial(network, address string) (net.Conn, error) {
	return net.Dial(network, address)
}

// DialUDP acts like Dial for UDP networks.
func (n *Net) DialUDP(network string, laddr, raddr *net.UDPAddr) (transport.UDPConn, error) {
	return net.DialUDP(network, laddr, raddr)
}

// ResolveIPAddr returns an address of IP end point.
func (n *Net) ResolveIPAddr(network, address string) (*net.IPAddr, error) {
	return net.ResolveIPAddr(network, address)
}

// ResolveUDPAddr returns an address of UDP end point.
func (n *Net) ResolveUDPAddr(network, address string) (*net.UDPAddr, error) {
	return net.ResolveUDPAddr(network, address)
}

type stdDialer struct {
	*net.Dialer
}

func (d stdDialer) Dial(network, address string) (net.Conn, error) {
	return d.Dialer.Dial(network, address)
}

// CreateDialer creates an instance of vnet.Dialer
func (n *Net) CreateDialer(d *net.Dialer) transport.Dialer {
	return stdDialer{d}
}
