// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
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

// EnqueueMessage copies buf into the batch. It returns the bytes taken (0 when
// the batch is full or buf empty), whether the batch is full afterwards and
// whether this message is the first in a batch that was empty.
func (m *messageBatch) EnqueueMessage(buf []byte, raddr net.Addr) (int, bool, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(buf) == 0 || m.isFull() {
		return 0, m.isFull(), false
	}
	wasEmpty := m.writePos == 0

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

	return len(buf), m.isFull(), wasEmpty
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

	writeReady chan struct{}
	// a message went into an empty write batch: the flusher arms its timer.
	// Buffered, so the signal is never lost; one slot is enough because every
	// later message of that batch is covered by the same timer.
	wake chan struct{}
	// closed and replaced each time the flusher swaps batches, so every waiting writer wakes at once
	writeBatchSwapped chan struct{}
	flusherDone       chan struct{}

	// wakeups counts the flusher's wake-ups (timer, full batch, first message)
	// for tests: an idle connection must not wake it.
	wakeups int32
	closed  int32
}

func newPingPong(size int, interval time.Duration, batchConn BatchPacketConn) *pingPong {
	pong := &pingPong{
		// buffered, so a batch that fills while the flusher is busy still gets its flush
		writeReady:        make(chan struct{}, 1),
		wake:              make(chan struct{}, 1),
		writeBatchSwapped: make(chan struct{}),
		flusherDone:       make(chan struct{}),
	}
	for i := range len(pong.batches) {
		pong.batches[i] = newMessageBatch(size, batchConn)
	}

	go pong.flusher(interval)

	return pong
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
		wasEmpty   bool
	)
	for {
		if writeBatch = p.getWriteBatch(); writeBatch != nil {
			n, isFull, wasEmpty = writeBatch.EnqueueMessage(buf, raddr)
			if n == len(buf) {
				break
			}
		}

		swapped := p.writeBatchSwapped
		p.mu.Unlock()
		<-swapped

		if atomic.LoadInt32(&p.closed) == 1 {
			return 0
		}

		p.mu.Lock()
	}
	p.mu.Unlock()

	// the first message of an empty write batch arms the flusher's timer
	if wasEmpty {
		signal(p.wake)
	}

	// enqueuing given message fills up the write batch, queue up a flush
	if isFull {
		signal(p.writeReady)
	}

	return n
}

// signal sends on a buffered signal channel without blocking: a signal
// already pending covers this one.
func signal(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
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

	close(p.writeBatchSwapped)
	p.writeBatchSwapped = make(chan struct{})

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
}

// flusher writes the write batch out when it fills (writeReady) or when its
// first message has waited interval (the timer). The timer is armed only by
// the first message of an empty write batch (wake) and stays unarmed after it
// fired, so a connection with nothing to send never wakes this goroutine. A
// message still leaves within interval of its enqueue: its own timer, an older
// one still running (which flushes whatever the write batch holds when it
// fires), or a full batch, which goes at once.
func (p *pingPong) flusher(interval time.Duration) {
	defer func() {
		// release writers waiting for a batch, they see closed and return
		p.mu.Lock()
		close(p.writeBatchSwapped)
		p.mu.Unlock()

		close(p.flusherDone)
	}()

	timer := time.NewTimer(interval)
	defer timer.Stop()
	if !timer.Stop() {
		<-timer.C
	}
	armed := false

	for atomic.LoadInt32(&p.closed) != 1 {
		var due <-chan time.Time
		if armed {
			due = timer.C
		}
		select {
		case <-p.wake:
			atomic.AddInt32(&p.wakeups, 1)
			if !armed {
				timer.Reset(interval)
				armed = true
			}

			continue
		case <-due:
			atomic.AddInt32(&p.wakeups, 1)
			armed = false
		case <-p.writeReady:
			atomic.AddInt32(&p.wakeups, 1)
		}

		readBatch := p.updateWriteBatchAndGetReadBatch()
		if readBatch == nil {
			continue
		}

		readBatch.Flush()
		p.updateReadBatch()
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
