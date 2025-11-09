// SPDX-FileCopyrightText: 2025 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package transport

import (
	"net"
	"time"
)

type PacketStream interface {
	net.Conn

	ReadWithAttributes(p []byte, attr *PacketAttributes) (n int, err error)
}

type PacketStreamPacketConn interface {
	net.PacketConn

	ReadFromWithAttributes(p []byte, attr *PacketAttributes) (n int, addr net.Addr, err error)
}

// NetConnToPacketStream wraps a net.Conn and implements the PacketStream interface by delegating
// calls to the underlying connection. ReadWithAttributes delegates to Read and
// ignores the provided PacketAttributes.
type NetConnToPacketStream struct {
	conn net.Conn
}

// NewNetConnToPacketStream returns a new Proxy that wraps the provided net.Conn.
func NewNetConnToPacketStream(conn net.Conn) *NetConnToPacketStream {
	return &NetConnToPacketStream{conn: conn}
}

// ReadWithAttributes reads from the underlying connection and ignores attr.
func (p *NetConnToPacketStream) ReadWithAttributes(b []byte, attr *PacketAttributes) (int, error) {
	return p.conn.Read(b)
}

// Delegate net.Conn methods to the underlying connection
func (p *NetConnToPacketStream) Read(b []byte) (int, error)        { return p.conn.Read(b) }
func (p *NetConnToPacketStream) Write(b []byte) (int, error)       { return p.conn.Write(b) }
func (p *NetConnToPacketStream) Close() error                      { return p.conn.Close() }
func (p *NetConnToPacketStream) LocalAddr() net.Addr               { return p.conn.LocalAddr() }
func (p *NetConnToPacketStream) RemoteAddr() net.Addr              { return p.conn.RemoteAddr() }
func (p *NetConnToPacketStream) SetDeadline(t time.Time) error     { return p.conn.SetDeadline(t) }
func (p *NetConnToPacketStream) SetReadDeadline(t time.Time) error { return p.conn.SetReadDeadline(t) }
func (p *NetConnToPacketStream) SetWriteDeadline(t time.Time) error {
	return p.conn.SetWriteDeadline(t)
}

type UDPConnToPacketStreamPacketConnAdapter struct {
	conn net.PacketConn
}

func NewUDPConnToPacketStreamPacketConnAdapter(conn net.PacketConn) *UDPConnToPacketStreamPacketConnAdapter {
	return &UDPConnToPacketStreamPacketConnAdapter{
		conn: conn,
	}
}

func (u *UDPConnToPacketStreamPacketConnAdapter) ReadFromWithAttributes(p []byte, attr *PacketAttributes) (n int, addr net.Addr, err error) {
	return u.conn.ReadFrom(p)
}

func (u *UDPConnToPacketStreamPacketConnAdapter) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	return u.conn.ReadFrom(p)
}

func (u *UDPConnToPacketStreamPacketConnAdapter) WriteTo(b []byte, addr net.Addr) (int, error) {
	n, err := u.conn.WriteTo(b, addr)
	return n, err
}

func (u *UDPConnToPacketStreamPacketConnAdapter) Close() error {
	return u.conn.Close()
}

func (u *UDPConnToPacketStreamPacketConnAdapter) LocalAddr() net.Addr {
	return u.conn.LocalAddr()
}

func (u *UDPConnToPacketStreamPacketConnAdapter) SetDeadline(t time.Time) error {
	return u.conn.SetDeadline(t)
}

func (u *UDPConnToPacketStreamPacketConnAdapter) SetReadDeadline(t time.Time) error {
	return u.conn.SetReadDeadline(t)
}

func (u *UDPConnToPacketStreamPacketConnAdapter) SetWriteDeadline(t time.Time) error {
	return u.conn.SetWriteDeadline(t)
}
