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

type NonBlockWritePacketConn interface {
	WriteToNonBlock(b []byte, addr net.Addr) (int, bool, error)
}

type packetConn struct {
	nextConn         net.PacketConn
	nextConnNonBlock NonBlockWritePacketConn
	closed           atomic.Bool
	closeOnce        sync.Once
	closeErr         error
	readMu           sync.Mutex
	writeMu          sync.Mutex
}

// NewPacketConn creates a new PacketConn wrapping the given net.PacketConn.
func NewPacketConn(pconn net.PacketConn) PacketConn {
	p := &packetConn{
		nextConn: pconn,
	}

	if v, ok := pconn.(NonBlockWritePacketConn); ok {
		p.nextConnNonBlock = v
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

	wg := sync.WaitGroup{}
	wg.Add(1)
	// Preserve existing deadlines until the context actually cancels the operation.
	detachDeadline := context.AfterFunc(ctx, func() {
		defer wg.Done()
		if err := p.nextConn.SetReadDeadline(veryOld); err != nil {
			_ = p.Close()
		}
	})

	n, raddr, err := p.nextConn.ReadFrom(b)

	var setDeadlineErr error
	if !detachDeadline() {
		wg.Wait()
		if !p.closed.Load() {
			setDeadlineErr = p.nextConn.SetReadDeadline(time.Time{})
		}
	}

	// Preserve I/O that has already made progress.
	if ctxErr := ctx.Err(); n == 0 && ctxErr != nil {
		err = ctxErr
	}

	if setDeadlineErr != nil {
		err = errors.Join(err, setDeadlineErr)
	}

	return n, raddr, err
}

// WriteToContext writes a packet with payload p to addr.
// Unlike net.PacketConn.WriteTo(), the provided context
// is used to control timeout.
// On packet-oriented connections, write timeouts are rare.
func (p *packetConn) WriteToContext(ctx context.Context, b []byte, raddr net.Addr) (int, error) { //nolint:cyclop
	p.writeMu.Lock()
	defer p.writeMu.Unlock()

	if p.closed.Load() {
		return 0, ErrClosing
	}
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}

	if p.nextConnNonBlock != nil {
		if n, ok, err := p.nextConnNonBlock.WriteToNonBlock(b, raddr); ok {
			return n, err
		}
	}

	wg := sync.WaitGroup{}
	wg.Add(1)
	// Preserve existing deadlines until the context actually cancels the operation.
	detachDeadline := context.AfterFunc(ctx, func() {
		defer wg.Done()
		if err := p.nextConn.SetWriteDeadline(veryOld); err != nil {
			_ = p.Close()
		}
	})

	n, err := p.nextConn.WriteTo(b, raddr)

	var setDeadlineErr error
	if !detachDeadline() {
		wg.Wait()
		if !p.closed.Load() {
			setDeadlineErr = p.nextConn.SetWriteDeadline(time.Time{})
		}
	}

	// Preserve I/O that has already made progress.
	if ctxErr := ctx.Err(); n == 0 && ctxErr != nil {
		err = ctxErr
	}

	if setDeadlineErr != nil {
		err = errors.Join(err, setDeadlineErr)
	}

	return n, err
}

// Close closes the connection.
// Any blocked ReadFromContext or WriteToContext operations will be unblocked
// and return errors.
func (p *packetConn) Close() error {
	p.closeOnce.Do(func() {
		p.closed.Store(true)
		p.closeErr = p.nextConn.Close()
	})

	return p.closeErr
}

// LocalAddr returns the local network address, if known.
func (p *packetConn) LocalAddr() net.Addr {
	return p.nextConn.LocalAddr()
}

// Conn returns the underlying net.PacketConn.
func (p *packetConn) Conn() net.PacketConn {
	return p.nextConn
}
