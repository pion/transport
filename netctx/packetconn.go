// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package netctx

import (
	"context"
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
	nextConn  net.PacketConn
	closed    chan struct{}
	closeOnce sync.Once
	readMu    sync.Mutex
	writeMu   sync.Mutex
}

// NewPacketConn creates a new PacketConn wrapping the given net.PacketConn.
func NewPacketConn(pconn net.PacketConn) PacketConn {
	p := &packetConn{
		nextConn: pconn,
		closed:   make(chan struct{}),
	}
	return p
}

func (p *packetConn) ReadFromContext(ctx context.Context, b []byte) (int, net.Addr, error) {
	p.readMu.Lock()
	defer p.readMu.Unlock()

	select {
	case <-p.closed:
		return 0, nil, net.ErrClosed
	default:
	}

	done := make(chan struct{})
	var wg sync.WaitGroup
	var errSetDeadline atomic.Value
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			// context canceled
			if err := p.nextConn.SetReadDeadline(veryOld); err != nil {
				errSetDeadline.Store(err)
				return
			}
			<-done
			if err := p.nextConn.SetReadDeadline(time.Time{}); err != nil {
				errSetDeadline.Store(err)
			}
		case <-done:
		}
	}()

	n, raddr, err := p.nextConn.ReadFrom(b)

	close(done)
	wg.Wait()
	if e := ctx.Err(); e != nil && n == 0 {
		err = e
	}
	if err2, ok := errSetDeadline.Load().(error); ok && err == nil && err2 != nil {
		err = err2
	}
	return n, raddr, err
}

func (p *packetConn) WriteToContext(ctx context.Context, b []byte, raddr net.Addr) (int, error) {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()

	select {
	case <-p.closed:
		return 0, ErrClosing
	default:
	}

	done := make(chan struct{})
	var wg sync.WaitGroup
	var errSetDeadline atomic.Value
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			// context canceled
			if err := p.nextConn.SetWriteDeadline(veryOld); err != nil {
				errSetDeadline.Store(err)
				return
			}
			<-done
			if err := p.nextConn.SetWriteDeadline(time.Time{}); err != nil {
				errSetDeadline.Store(err)
			}
		case <-done:
		}
	}()

	n, err := p.nextConn.WriteTo(b, raddr)

	close(done)
	wg.Wait()
	if e := ctx.Err(); e != nil && n == 0 {
		err = e
	}
	if err2, ok := errSetDeadline.Load().(error); ok && err == nil && err2 != nil {
		err = err2
	}
	return n, err
}

func (p *packetConn) Close() error {
	err := p.nextConn.Close()
	p.closeOnce.Do(func() {
		p.writeMu.Lock()
		p.readMu.Lock()
		close(p.closed)
		p.readMu.Unlock()
		p.writeMu.Unlock()
	})
	return err
}

func (p *packetConn) LocalAddr() net.Addr {
	return p.nextConn.LocalAddr()
}

func (p *packetConn) Conn() net.PacketConn {
	return p.nextConn
}
