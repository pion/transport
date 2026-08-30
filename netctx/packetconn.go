// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package netctx

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// ReaderFrom is an interface for context controlled packet reader.
type ReaderFrom interface {
	ReadFromContext(context.Context, []byte) (int, net.Addr, error)
}

// WriterTo is an interface for context controlled packet writer.
type WriterTo interface {
	WriteToContext(context.Context, []byte, net.Addr) (int, error)
}

// PacketConn is a wrapper of net.PacketConn using context.Context.
type PacketConn interface {
	ReaderFrom
	WriterTo
	io.Closer
	LocalAddr() net.Addr
	Conn() net.PacketConn
}

type packetConn struct {
	nextConn net.PacketConn
	closed   atomic.Bool
	readMu   sync.Mutex
	writeMu  sync.Mutex
}

// NewPacketConn creates a new PacketConn wrapping the given net.PacketConn.
func NewPacketConn(pconn net.PacketConn) PacketConn {
	p := &packetConn{
		nextConn: pconn,
	}

	return p
}

// ReadFromContext reads a packet from the connection,
// copying the payload into p. It returns the number of
// bytes copied into p and the return address that
// was on the packet.
// It returns the number of bytes read (0 <= n <= len(p))
// and any error encountered. Callers should always process
// the n > 0 bytes returned before considering the error err.
// Unlike net.PacketConn.ReadFrom(), the provided context is
// used to control timeout.
func (p *packetConn) ReadFromContext(ctx context.Context, b []byte) (int, net.Addr, error) {
	p.readMu.Lock()
	defer p.readMu.Unlock()

	if p.closed.Load() {
		return 0, nil, net.ErrClosed
	}
	if ctx.Err() != nil {
		return 0, nil, ctx.Err()
	}

	if deadline, ok := ctx.Deadline(); ok {
		if err := p.nextConn.SetReadDeadline(deadline); err != nil {
			return 0, nil, err
		}
	}

	detachDeadline := context.AfterFunc(ctx, func() {
		if err := p.nextConn.SetReadDeadline(veryOld); err != nil {
			_ = p.nextConn.Close()
		}
	})

	n, raddr, err := p.nextConn.ReadFrom(b)

	detachDeadline()

	var setDeadlineErr error
	if !p.closed.Load() {
		setDeadlineErr = p.nextConn.SetReadDeadline(time.Time{})
	}

	return n, raddr, errors.Join(err, ctx.Err(), setDeadlineErr)
}

// WriteToContext writes a packet with payload p to addr.
// Unlike net.PacketConn.WriteTo(), the provided context
// is used to control timeout.
// On packet-oriented connections, write timeouts are rare.
func (p *packetConn) WriteToContext(ctx context.Context, b []byte, raddr net.Addr) (int, error) {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()

	if p.closed.Load() {
		return 0, ErrClosing
	}
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}

	if deadline, ok := ctx.Deadline(); ok {
		if err := p.nextConn.SetWriteDeadline(deadline); err != nil {
			return 0, err
		}
	}

	detachDeadline := context.AfterFunc(ctx, func() {
		if errors.Is(ctx.Err(), context.Canceled) {
			if err := p.nextConn.SetWriteDeadline(veryOld); err != nil {
				_ = p.nextConn.Close()
			}
		}
	})

	n, err := p.nextConn.WriteTo(b, raddr)

	detachDeadline()
	var setDeadlineErr error
	if !p.closed.Load() {
		setDeadlineErr = p.nextConn.SetWriteDeadline(time.Time{})
	}

	return n, errors.Join(ctx.Err(), setDeadlineErr, err)
}

// Close closes the connection.
// Any blocked ReadFromContext or WriteToContext operations will be unblocked
// and return errors.
func (p *packetConn) Close() error {
	if !p.closed.Swap(true) {
		return p.nextConn.Close()
	}

	return nil
}

// LocalAddr returns the local network address, if known.
func (p *packetConn) LocalAddr() net.Addr {
	return p.nextConn.LocalAddr()
}

// Conn returns the underlying net.PacketConn.
func (p *packetConn) Conn() net.PacketConn {
	return p.nextConn
}
