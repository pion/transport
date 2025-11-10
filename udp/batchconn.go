// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package udp

import (
	"io"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

// BatchWriter represents conn can write messages in batch.
type BatchWriter interface {
	WriteBatch(ms []ipv4.Message, flags int) (int, error)
}

// BatchReader represents conn can read messages in batch.
type BatchReader interface {
	ReadBatch(msg []ipv4.Message, flags int) (int, error)
}

// BatchPacketConn represents conn can read/write messages in batch.
type BatchPacketConn interface {
	BatchWriter
	BatchReader
	io.Closer
}

// ---------------------------------

type messageBatch struct {
	size      int
	batchConn BatchPacketConn

	mu       sync.Mutex
	messages []ipv4.Message
	writePos int
}

func newMessageBatch(size int, batchConn BatchPacketConn) *messageBatch {
	m := &messageBatch{
		size:      size,
		batchConn: batchConn,
	}
	m.init()

	return m
}

func (m *messageBatch) init() {
	m.messages = make([]ipv4.Message, m.size)
	for i := range m.messages {
		m.messages[i].Buffers = [][]byte{make([]byte, sendMTU)}
	}
}

func (m *messageBatch) isFull() bool {
	return m.writePos == m.size
}

func (m *messageBatch) EnqueueMessage(buf []byte, raddr net.Addr) (int, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(buf) == 0 || m.isFull() {
		return 0, m.isFull()
	}

	msg := &m.messages[m.writePos]
	// reset buffers
	msg.Buffers = msg.Buffers[:1]
	msg.Buffers[0] = msg.Buffers[0][:cap(msg.Buffers[0])]

	if raddr != nil {
		msg.Addr = raddr
	}
	if n := copy(msg.Buffers[0], buf); n < len(buf) {
		extraBuffer := make([]byte, len(buf)-n)
		copy(extraBuffer, buf[n:])
		msg.Buffers = append(msg.Buffers, extraBuffer)
	} else {
		msg.Buffers[0] = msg.Buffers[0][:n]
	}
	m.writePos++

	return len(buf), m.isFull()
}

func (m *messageBatch) Flush() {
	m.mu.Lock()
	defer m.mu.Unlock()

	var txN int
	for txN < m.writePos {
		n, err := m.batchConn.WriteBatch(m.messages[txN:m.writePos], 0)
		if err != nil {
			break
		}
		txN += n
	}

	m.writePos = 0
}

// ---------------------------------

type pingPong struct {
	mu            sync.Mutex
	batches       [2]*messageBatch
	writeBatchIdx int
	readBatchIdx  int
	flushPending  bool

	writeReady     chan struct{}
	flushCycleDone chan struct{}
	flusherDone    chan struct{}

	closed int32
}

func newPingPong(size int, interval time.Duration, batchConn BatchPacketConn) *pingPong {
	p := &pingPong{
		writeReady:     make(chan struct{}),
		flushCycleDone: make(chan struct{}),
		flusherDone:    make(chan struct{}),
	}
	for i := 0; i < len(p.batches); i++ {
		p.batches[i] = newMessageBatch(size, batchConn)
	}

	go p.flusher(interval)

	return p
}

func (p *pingPong) Close() {
	atomic.StoreInt32(&p.closed, 1)

	select {
	case p.writeReady <- struct{}{}:
	default:
	}

	<-p.flusherDone
}

func (p *pingPong) EnqueueMessage(buf []byte, raddr net.Addr) int {
	p.mu.Lock()
	var (
		writeBatch *messageBatch
		n          int
		isFull     bool
	)
	for {
		if writeBatch = p.getWriteBatch(); writeBatch != nil {
			n, isFull = writeBatch.EnqueueMessage(buf, raddr)
			if n == len(buf) {
				break
			}
		}

		p.mu.Unlock()
		select {
		case <-p.flushCycleDone:
		case <-time.After(100 * time.Microsecond):
		}

		if atomic.LoadInt32(&p.closed) == 1 {
			return 0
		}

		p.mu.Lock()
	}
	p.mu.Unlock()

	// enqueuing given message fills up the write batch, queue up a flush
	if isFull {
		select {
		case p.writeReady <- struct{}{}:
		default:
		}
	}

	return n
}

func (p *pingPong) getWriteBatch() *messageBatch {
	if p.writeBatchIdx == p.readBatchIdx && p.flushPending {
		return nil
	}

	return p.batches[p.writeBatchIdx]
}

func (p *pingPong) updateWriteBatchAndGetReadBatch() *messageBatch {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.writeBatchIdx != p.readBatchIdx || p.flushPending {
		return nil
	}

	p.writeBatchIdx ^= 1
	p.flushPending = true

	return p.batches[p.readBatchIdx]
}

func (p *pingPong) updateReadBatch() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.flushPending {
		return
	}

	p.readBatchIdx ^= 1
	p.flushPending = false

	select {
	case p.flushCycleDone <- struct{}{}:
	default:
	}
}

func (p *pingPong) flusher(interval time.Duration) {
	defer close(p.flusherDone)

	writeTicker := time.NewTicker(interval / 2)
	defer writeTicker.Stop()

	lastFlushAt := time.Now().Add(-interval)
	for atomic.LoadInt32(&p.closed) != 1 {
		select {
		case <-writeTicker.C:
			if time.Since(lastFlushAt) < interval {
				continue
			}
		case <-p.writeReady:
		}

		readBatch := p.updateWriteBatchAndGetReadBatch()
		if readBatch == nil {
			continue
		}

		readBatch.Flush()
		p.updateReadBatch()

		lastFlushAt = time.Now()
	}
}

// ------------------------------

// BatchConn uses ipv4/v6.NewPacketConn to wrap a net.PacketConn to write/read messages in batch,
// only available in linux. In other platform, it will use single Write/Read as same as net.Conn.
type BatchConn struct {
	net.PacketConn

	batchConn BatchPacketConn

	// ping-pong the batches to be able to accept new packets while a batch is written to socket
	batchPingPong *pingPong
}

// NewBatchConn creates a *BatchConn from net.PacketConn with batch configs.
func NewBatchConn(conn net.PacketConn, batchWriteSize int, batchWriteInterval time.Duration) *BatchConn {
	bc := &BatchConn{
		PacketConn: conn,
	}

	// batch write only supports linux
	if runtime.GOOS == "linux" {
		if pc4 := ipv4.NewPacketConn(conn); pc4 != nil {
			bc.batchConn = pc4
		} else if pc6 := ipv6.NewPacketConn(conn); pc6 != nil {
			bc.batchConn = pc6
		}

		bc.batchPingPong = newPingPong(batchWriteSize, batchWriteInterval, bc.batchConn)
	}

	return bc
}

// Close batchConn and the underlying PacketConn.
func (c *BatchConn) Close() error {
	if c.batchPingPong != nil {
		c.batchPingPong.Close()
	}

	if c.batchConn != nil {
		return c.batchConn.Close()
	}

	return c.PacketConn.Close()
}

// WriteTo write message to an UDPAddr, addr should be nil if it is a connected socket.
func (c *BatchConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	if c.batchConn == nil {
		return c.PacketConn.WriteTo(b, addr)
	}

	return c.batchPingPong.EnqueueMessage(b, addr), nil
}

// ReadBatch reads messages in batch, return length of message readed and error.
func (c *BatchConn) ReadBatch(msgs []ipv4.Message, flags int) (int, error) {
	if c.batchConn == nil {
		n, addr, err := c.PacketConn.ReadFrom(msgs[0].Buffers[0])
		if err == nil {
			msgs[0].N = n
			msgs[0].Addr = addr

			return 1, nil
		}

		return 0, err
	}

	return c.batchConn.ReadBatch(msgs, flags)
}
