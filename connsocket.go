// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package transport

import (
	"net"
	"time"
)

type NetConnSocket interface {
	net.Conn

	ReadWithAttributes(p []byte, attr *PacketAttributes) (n int, err error)
}

type PacketConnSocket interface {
	net.PacketConn

	ReadFromWithAttributes(p []byte, attr *PacketAttributes) (n int, addr net.Addr, err error)
}

// NetConnToNetConnSocket wraps a net.Conn and implements the PacketStream interface by delegating
// calls to the underlying connection. ReadWithAttributes delegates to Read and
// ignores the provided PacketAttributes.
type NetConnToNetConnSocket struct {
	conn net.Conn
}

// NewNetConnToNetConnSocket returns a new Proxy that wraps the provided net.Conn.
func NewNetConnToNetConnSocket(conn net.Conn) *NetConnToNetConnSocket {
	return &NetConnToNetConnSocket{conn: conn}
}

// ReadWithAttributes reads from the underlying connection and ignores attributes.
func (p *NetConnToNetConnSocket) ReadWithAttributes(b []byte, _ *PacketAttributes) (int, error) {
	return p.conn.Read(b)
}

// Delegate net.Conn methods to the underlying connection.
func (p *NetConnToNetConnSocket) Read(b []byte) (int, error)        { return p.conn.Read(b) }
func (p *NetConnToNetConnSocket) Write(b []byte) (int, error)       { return p.conn.Write(b) }
func (p *NetConnToNetConnSocket) Close() error                      { return p.conn.Close() }
func (p *NetConnToNetConnSocket) LocalAddr() net.Addr               { return p.conn.LocalAddr() }
func (p *NetConnToNetConnSocket) RemoteAddr() net.Addr              { return p.conn.RemoteAddr() }
func (p *NetConnToNetConnSocket) SetDeadline(t time.Time) error     { return p.conn.SetDeadline(t) }
func (p *NetConnToNetConnSocket) SetReadDeadline(t time.Time) error { return p.conn.SetReadDeadline(t) }
func (p *NetConnToNetConnSocket) SetWriteDeadline(t time.Time) error {
	return p.conn.SetWriteDeadline(t)
}
