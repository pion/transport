// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package netchange

import (
	"net"

	"github.com/pion/transport/v5"
)

var _ transport.Net = (*Detector)(nil)

// Interfaces returns the latest list of system network interfaces.
func (d *Detector) Interfaces() ([]*transport.Interface, error) {
	return d.network.Load().Interfaces()
}

// InterfaceByIndex looks up an index in the latest network snapshot.
func (d *Detector) InterfaceByIndex(index int) (*transport.Interface, error) {
	return d.network.Load().InterfaceByIndex(index)
}

// InterfaceByName looks up a name in the latest network snapshot.
func (d *Detector) InterfaceByName(name string) (*transport.Interface, error) {
	return d.network.Load().InterfaceByName(name)
}

// ListenPacket announces on the local network address.
func (d *Detector) ListenPacket(network, address string) (net.PacketConn, error) {
	return d.network.Load().ListenPacket(network, address)
}

// ListenUDP acts like ListenPacket for UDP networks.
func (d *Detector) ListenUDP(network string, laddr *net.UDPAddr) (transport.UDPConn, error) {
	return d.network.Load().ListenUDP(network, laddr)
}

// ListenTCP announces on the local TCP address.
func (d *Detector) ListenTCP(network string, laddr *net.TCPAddr) (transport.TCPListener, error) {
	return d.network.Load().ListenTCP(network, laddr)
}

// Dial connects to the address on the named network.
func (d *Detector) Dial(network, address string) (net.Conn, error) {
	return d.network.Load().Dial(network, address)
}

// DialUDP acts like Dial for UDP networks.
func (d *Detector) DialUDP(network string, laddr, raddr *net.UDPAddr) (transport.UDPConn, error) {
	return d.network.Load().DialUDP(network, laddr, raddr)
}

// DialTCP acts like Dial for TCP networks.
func (d *Detector) DialTCP(network string, laddr, raddr *net.TCPAddr) (transport.TCPConn, error) {
	return d.network.Load().DialTCP(network, laddr, raddr)
}

// ResolveIPAddr returns an address of an IP end point.
func (d *Detector) ResolveIPAddr(network, address string) (*net.IPAddr, error) {
	return d.network.Load().ResolveIPAddr(network, address)
}

// ResolveUDPAddr returns an address of a UDP end point.
func (d *Detector) ResolveUDPAddr(network, address string) (*net.UDPAddr, error) {
	return d.network.Load().ResolveUDPAddr(network, address)
}

// ResolveTCPAddr returns an address of a TCP end point.
func (d *Detector) ResolveTCPAddr(network, address string) (*net.TCPAddr, error) {
	return d.network.Load().ResolveTCPAddr(network, address)
}

// CreateDialer creates a dialer using the standard net package.
func (d *Detector) CreateDialer(dialer *net.Dialer) transport.Dialer {
	return d.network.Load().CreateDialer(dialer)
}

// CreateListenConfig creates a listen configuration using the standard net package.
func (d *Detector) CreateListenConfig(config *net.ListenConfig) transport.ListenConfig {
	return d.network.Load().CreateListenConfig(config)
}
