// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package packetio

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/transport/v3/test"
	"github.com/stretchr/testify/assert"
)

func TestBuffer(t *testing.T) {
	assert := assert.New(t)

	buffer := NewBuffer()
	packet := make([]byte, 4)

	// Write once
	n, err := buffer.Write([]byte{0, 1})
	assert.NoError(err)
	assert.Equal(2, n)

	// Read once
	n, err = buffer.Read(packet)
	assert.NoError(err)
	assert.Equal(2, n)
	assert.Equal([]byte{0, 1}, packet[:n])

	// Read deadline
	err = buffer.SetReadDeadline(time.Unix(0, 1))
	assert.NoError(err)
	n, err = buffer.Read(packet)
	var e net.Error
	assert.ErrorAs(err, &e)
	assert.True(e.Timeout())
	assert.Equal(0, n)

	// Reset deadline
	err = buffer.SetReadDeadline(time.Time{})
	assert.NoError(err)

	// Write twice
	n, err = buffer.Write([]byte{2, 3, 4})
	assert.NoError(err)
	assert.Equal(3, n)

	n, err = buffer.Write([]byte{5, 6, 7})
	assert.NoError(err)
	assert.Equal(3, n)

	// Read twice
	n, err = buffer.Read(packet)
	assert.NoError(err)
	assert.Equal(3, n)
	assert.Equal([]byte{2, 3, 4}, packet[:n])

	n, err = buffer.Read(packet)
	assert.NoError(err)
	assert.Equal(3, n)
	assert.Equal([]byte{5, 6, 7}, packet[:n])

	// Write once prior to close.
	_, err = buffer.Write([]byte{3})
	assert.NoError(err)

	// Close
	err = buffer.Close()
	assert.NoError(err)

	// Future writes will error
	_, err = buffer.Write([]byte{4})
	assert.Error(err)

	// But we can read the remaining data.
	n, err = buffer.Read(packet)
	assert.NoError(err)
	assert.Equal(1, n)
	assert.Equal([]byte{3}, packet[:n])

	// Until EOF
	_, err = buffer.Read(packet)
	assert.Equal(io.EOF, err)
}

func testWraparound(t *testing.T, grow bool) {
	t.Helper()

	assert := assert.New(t)

	buffer := NewBuffer()
	err := buffer.grow()
	assert.NoError(err)

	buffer.head = len(buffer.data) - 13
	buffer.tail = buffer.head

	p1 := []byte{1, 2, 3}
	p2 := []byte{4, 5, 6}
	p3 := []byte{7, 8, 9}
	p4 := []byte{10, 11, 12}

	_, err = buffer.Write(p1)
	assert.NoError(err)
	_, err = buffer.Write(p2)
	assert.NoError(err)
	_, err = buffer.Write(p3)
	assert.NoError(err)

	packet := make([]byte, 10)

	n, err := buffer.Read(packet)
	assert.NoError(err)
	assert.Equal(p1, packet[:n])

	if grow {
		err = buffer.grow()
		assert.NoError(err)
	}

	n, err = buffer.Read(packet)
	assert.NoError(err)
	assert.Equal(p2, packet[:n])

	_, err = buffer.Write(p4)
	assert.NoError(err)

	n, err = buffer.Read(packet)
	assert.NoError(err)
	assert.Equal(p3, packet[:n])
	n, err = buffer.Read(packet)
	assert.NoError(err)
	assert.Equal(p4, packet[:n])

	if !grow {
		assert.Equal(len(buffer.data), minSize)
	} else {
		assert.Equal(len(buffer.data), 2*minSize)
	}
}

func TestBufferWraparound(t *testing.T) {
	testWraparound(t, false)
}

func TestBufferWraparoundGrow(t *testing.T) {
	testWraparound(t, true)
}

func TestBufferAsync(t *testing.T) {
	assert := assert.New(t)

	buffer := NewBuffer()

	// Start up a goroutine to start a blocking read.
	done := make(chan struct{})
	go func() {
		packet := make([]byte, 4)

		n, err := buffer.Read(packet)
		assert.NoError(err)
		assert.Equal(2, n)
		assert.Equal([]byte{0, 1}, packet[:n])

		_, err = buffer.Read(packet)
		assert.Equal(io.EOF, err)

		close(done)
	}()

	// Wait for the reader to start reading.
	time.Sleep(time.Millisecond)

	// Write once
	n, err := buffer.Write([]byte{0, 1})
	assert.NoError(err)
	assert.Equal(2, n)

	// Wait for the reader to start reading again.
	time.Sleep(time.Millisecond)

	// Close will unblock the reader.
	err = buffer.Close()
	assert.NoError(err)

	<-done
}

func TestBufferLimitCount(t *testing.T) {
	assert := assert.New(t)

	buffer := NewBuffer()
	buffer.SetLimitCount(2)

	assert.Equal(0, buffer.Count())

	// Write twice
	n, err := buffer.Write([]byte{0, 1})
	assert.NoError(err)
	assert.Equal(2, n)
	assert.Equal(1, buffer.Count())

	n, err = buffer.Write([]byte{2, 3})
	assert.NoError(err)
	assert.Equal(2, n)
	assert.Equal(2, buffer.Count())

	// Over capacity
	_, err = buffer.Write([]byte{4, 5})
	assert.Equal(ErrFull, err)
	assert.Equal(2, buffer.Count())

	// Read once
	packet := make([]byte, 4)
	n, err = buffer.Read(packet)
	assert.NoError(err)
	assert.Equal(2, n)
	assert.Equal([]byte{0, 1}, packet[:n])
	assert.Equal(1, buffer.Count())

	// Write once
	n, err = buffer.Write([]byte{6, 7})
	assert.NoError(err)
	assert.Equal(2, n)
	assert.Equal(2, buffer.Count())

	// Over capacity
	_, err = buffer.Write([]byte{8, 9})
	assert.Equal(ErrFull, err)
	assert.Equal(2, buffer.Count())

	// Read twice
	n, err = buffer.Read(packet)
	assert.NoError(err)
	assert.Equal(2, n)
	assert.Equal([]byte{2, 3}, packet[:n])
	assert.Equal(1, buffer.Count())

	n, err = buffer.Read(packet)
	assert.NoError(err)
	assert.Equal(2, n)
	assert.Equal([]byte{6, 7}, packet[:n])
	assert.Equal(0, buffer.Count())

	// Nothing left.
	err = buffer.Close()
	assert.NoError(err)
}

func TestBufferLimitSize(t *testing.T) {
	assert := assert.New(t)

	buffer := NewBuffer()
	buffer.SetLimitSize(11)

	assert.Equal(0, buffer.Size())

	// Write twice
	n, err := buffer.Write([]byte{0, 1})
	assert.NoError(err)
	assert.Equal(2, n)
	assert.Equal(4, buffer.Size())

	n, err = buffer.Write([]byte{2, 3})
	assert.NoError(err)
	assert.Equal(2, n)
	assert.Equal(8, buffer.Size())

	// Over capacity
	_, err = buffer.Write([]byte{4, 5})
	assert.Equal(ErrFull, err)
	assert.Equal(8, buffer.Size())

	// Cheeky write at exact size.
	n, err = buffer.Write([]byte{6})
	assert.NoError(err)
	assert.Equal(1, n)
	assert.Equal(11, buffer.Size())

	// Read once
	packet := make([]byte, 4)
	n, err = buffer.Read(packet)
	assert.NoError(err)
	assert.Equal(2, n)
	assert.Equal([]byte{0, 1}, packet[:n])
	assert.Equal(7, buffer.Size())

	// Write once
	n, err = buffer.Write([]byte{7, 8})
	assert.NoError(err)
	assert.Equal(2, n)
	assert.Equal(11, buffer.Size())

	// Over capacity
	_, err = buffer.Write([]byte{9, 10})
	assert.Equal(ErrFull, err)
	assert.Equal(11, buffer.Size())

	// Read everything
	n, err = buffer.Read(packet)
	assert.NoError(err)
	assert.Equal(2, n)
	assert.Equal([]byte{2, 3}, packet[:n])
	assert.Equal(7, buffer.Size())

	n, err = buffer.Read(packet)
	assert.NoError(err)
	assert.Equal(1, n)
	assert.Equal([]byte{6}, packet[:n])
	assert.Equal(4, buffer.Size())

	n, err = buffer.Read(packet)
	assert.NoError(err)
	assert.Equal(2, n)
	assert.Equal([]byte{7, 8}, packet[:n])
	assert.Equal(0, buffer.Size())

	// Nothing left.
	err = buffer.Close()
	assert.NoError(err)
}

func TestBufferLimitSizes(t *testing.T) {
	if sizeHardLimit {
		t.Skip("skipping since packetioSizeHardLimit is enabled")
	}
	sizes := []int{
		128 * 1024,
		1024 * 1024,
		8 * 1024 * 1024,
		0, // default
	}
	const headerSize = 2
	const packetSize = 0x8000

	for _, size := range sizes {
		size := size
		name := "default"
		if size > 0 {
			name = fmt.Sprintf("%dkBytes", size/1024)
		}

		t.Run(name, func(t *testing.T) {
			assert := assert.New(t)

			buffer := NewBuffer()
			if size == 0 {
				size = maxSize
			} else {
				buffer.SetLimitSize(size + headerSize)
			}
			now := time.Now()
			assert.NoError(buffer.SetReadDeadline(now.Add(5 * time.Second))) // Set deadline to avoid test deadlock

			nPackets := size / (packetSize + headerSize)

			for i := 0; i < nPackets; i++ {
				_, err := buffer.Write(make([]byte, packetSize))
				assert.NoError(err)
			}

			// Next write is expected to be errored.
			_, err := buffer.Write(make([]byte, packetSize))
			assert.Error(err, ErrFull)

			packet := make([]byte, size)
			for i := 0; i < nPackets; i++ {
				n, err := buffer.Read(packet)
				assert.NoError(err)
				assert.Equal(packetSize, n)
				if err != nil {
					assert.FailNow("Read failed", err)
				}
			}
		})
	}
}

func TestBufferMisc(t *testing.T) {
	assert := assert.New(t)

	buffer := NewBuffer()

	// Write once
	n, err := buffer.Write([]byte{0, 1, 2, 3})
	assert.NoError(err)
	assert.Equal(4, n)

	// Try to read with a short buffer
	packet := make([]byte, 3)
	_, err = buffer.Read(packet)
	assert.Equal(io.ErrShortBuffer, err)

	// Close
	err = buffer.Close()
	assert.NoError(err)

	// Make sure you can Close twice
	err = buffer.Close()
	assert.NoError(err)
}

var errTooManyCallOfGetBuffer = errors.New("too many call of getBuffer")

func TestBufferAlloc(t *testing.T) {
	packet := make([]byte, 1024)

	const countTolerance = 1

	test := func(fn func(func() (*Buffer, error), int, *error) func(), count int, maxVal float64) func(t *testing.T) {
		return func(t *testing.T) {
			t.Helper()

			const nRuns = 100

			// Create buffers in advance to avoid measuring allocs in NewBuffer()
			// +1 buffer for warm-up run
			buffers := make([]*Buffer, 0, nRuns+1)
			for i := 0; i < nRuns+1; i++ {
				buffers = append(buffers, NewBuffer())
			}
			var iBuffer int
			getBuffer := func() (*Buffer, error) {
				if iBuffer >= len(buffers) {
					return nil, errTooManyCallOfGetBuffer
				}
				ret := buffers[iBuffer]
				iBuffer++

				return ret, nil
			}

			var err error
			// AllocsPerRun calls the func once as a warm-up and then call it specified times
			allocs := testing.AllocsPerRun(nRuns, fn(getBuffer, count, &err))
			assert.NoError(t, err)
			assert.LessOrEqualf(t, allocs, maxVal+countTolerance,
				"count=%d, max=%f+%d, got %f", count, maxVal, countTolerance, allocs)
		}
	}

	// Write (1024+2)*count bytes
	writer := func(getBuffer func() (*Buffer, error), count int, errOut *error) func() {
		return func() {
			// Call only buffer.Write() on the non-error paths to avoid wrong count of allocs
			buffer, err := getBuffer() // getBuffer doesn't alloc
			if err != nil {
				*errOut = err

				return
			}
			for i := 0; i < count; i++ {
				if _, err := buffer.Write(packet); err != nil {
					*errOut = fmt.Errorf("write: %w", err)

					return
				}
			}
		}
	}

	// Buffer size will be grown as
	// 2048 -> 4096 -> 8192 -> 16384 -> 32768 -> 65536 -> 131072 -> 163840 -> 204800
	//   -> 256000 -> 320000 -> 400000 -> 500000 -> 625000 -> 781250 -> 976562 -> 1220702
	// based on the logic in Buffer.grow()
	t.Run("10 writes", test(writer, 10, 4))      // 10260 bytes
	t.Run("100 writes", test(writer, 100, 7))    // 102600 bytes
	t.Run("200 writes", test(writer, 200, 10))   // 205200 bytes
	t.Run("400 writes", test(writer, 400, 13))   // 410400 bytes
	t.Run("1000 writes", test(writer, 1000, 17)) // 1026000 bytes

	// Read and write same times, so the buffer size should never grow
	wr := func(getBuffer func() (*Buffer, error), count int, errOut *error) func() {
		return func() {
			// Call only buffer.Write() on the non-error paths to avoid wrong count of allocs
			buffer, err := getBuffer() // getBuffer doesn't alloc
			if err != nil {
				*errOut = err

				return
			}
			for i := 0; i < count; i++ {
				if _, err := buffer.Write(packet); err != nil {
					*errOut = fmt.Errorf("write: %w", err)

					return
				}
				if _, err := buffer.Read(packet); err != nil {
					*errOut = fmt.Errorf("read: %w", err)

					return
				}
			}
		}
	}

	t.Run("10 writes and reads", test(wr, 10, 1))
	t.Run("100 writes and reads", test(wr, 100, 1))
	t.Run("1000 writes and reads", test(wr, 1000, 1))
	t.Run("10000 writes and reads", test(wr, 10000, 1))
}

func benchmarkBufferWR(b *testing.B, size int64, write bool, grow int) { // nolint:unparam
	b.Helper()
	buffer := NewBuffer()
	packet := make([]byte, size)

	// Grow the buffer first
	pad := make([]byte, 1022)
	for buffer.Size() < grow {
		_, err := buffer.Write(pad)
		if err != nil {
			b.Fatalf("Write: %v", err)
		}
	}
	for buffer.Size() > 0 {
		_, err := buffer.Read(pad)
		if err != nil {
			b.Fatalf("Write: %v", err)
		}
	}

	if write {
		_, err := buffer.Write(packet)
		if err != nil {
			b.Fatalf("Write: %v", err)
		}
	}

	b.SetBytes(size)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := buffer.Write(packet)
		if err != nil {
			b.Fatalf("Write: %v", err)
		}
		_, err = buffer.Read(packet)
		if err != nil {
			b.Fatalf("Read: %v", err)
		}
	}
}

// In this benchmark, the buffer is often empty, which is hopefully
// typical of real usage.
func BenchmarkBufferWR14(b *testing.B) {
	benchmarkBufferWR(b, 14, false, 128000)
}

func BenchmarkBufferWR140(b *testing.B) {
	benchmarkBufferWR(b, 140, false, 128000)
}

func BenchmarkBufferWR1400(b *testing.B) {
	benchmarkBufferWR(b, 1400, false, 128000)
}

// Here, the buffer never becomes empty, which forces wraparound.
func BenchmarkBufferWWR14(b *testing.B) {
	benchmarkBufferWR(b, 14, true, 128000)
}

func BenchmarkBufferWWR140(b *testing.B) {
	benchmarkBufferWR(b, 140, true, 128000)
}

func BenchmarkBufferWWR1400(b *testing.B) {
	benchmarkBufferWR(b, 1400, true, 128000)
}

func benchmarkBuffer(b *testing.B, size int64) {
	b.Helper()

	buffer := NewBuffer()
	b.SetBytes(size)

	done := make(chan struct{})
	go func() {
		packet := make([]byte, size)

		for {
			_, err := buffer.Read(packet)
			if errors.Is(err, io.EOF) {
				break
			} else if err != nil {
				b.Error(err)

				break
			}
		}

		close(done)
	}()

	packet := make([]byte, size)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var err error
		for {
			_, err = buffer.Write(packet)
			if !errors.Is(err, ErrFull) {
				break
			}
			time.Sleep(time.Microsecond)
		}
		if err != nil {
			b.Fatal(err)
		}
	}

	err := buffer.Close()
	if err != nil {
		b.Fatal(err)
	}

	<-done
}

func BenchmarkBuffer14(b *testing.B) {
	benchmarkBuffer(b, 14)
}

func BenchmarkBuffer140(b *testing.B) {
	benchmarkBuffer(b, 140)
}

func BenchmarkBuffer1400(b *testing.B) {
	benchmarkBuffer(b, 1400)
}

func TestBufferConcurrentRead(t *testing.T) {
	assert := assert.New(t)

	buffer := NewBuffer()
	packet := make([]byte, 4)

	// Write twice
	n, err := buffer.Write([]byte{2, 3, 4})
	assert.NoError(err)
	assert.Equal(3, n)

	n, err = buffer.Write([]byte{5, 6, 7})
	assert.NoError(err)
	assert.Equal(3, n)

	// Read twice
	n, err = buffer.Read(packet)
	assert.NoError(err)
	assert.Equal(3, n)
	assert.Equal([]byte{2, 3, 4}, packet[:n])

	n, err = buffer.Read(packet)
	assert.NoError(err)
	assert.Equal(3, n)
	assert.Equal([]byte{5, 6, 7}, packet[:n])

	errCh := make(chan error, 2)
	readIntoErr := func() {
		packet := make([]byte, 4)
		_, readErr := buffer.Read(packet)
		errCh <- readErr
	}
	go readIntoErr()
	go readIntoErr()

	// Close
	err = buffer.Close()
	assert.NoError(err)

	err = <-errCh
	assert.Equal(io.EOF, err)
	err = <-errCh
	assert.Equal(io.EOF, err)
}

func TestBufferConcurrentReadWrite(t *testing.T) {
	defer test.TimeOut(time.Second * 5).Stop()

	assert := assert.New(t)

	buffer := NewBuffer()

	numPkts := 1000
	var numRead uint64
	allRead := make(chan struct{})
	readPkts := func(count int) {
		packet := make([]byte, 4)
		for i := 0; i < count; i++ {
			_, readErr := buffer.Read(packet)
			if readErr != nil {
				return
			}
			if atomic.AddUint64(&numRead, 1) == uint64(numPkts) { //nolint:gosec
				close(allRead)

				return
			}
		}
	}
	go readPkts(numPkts)
	go readPkts(numPkts / 100)

	for i := 0; i < numPkts; i++ {
		_, writeErr := buffer.Write([]byte{2, 3, 4})
		assert.NoError(writeErr)
	}
	<-allRead

	assert.NoError(buffer.Close())
}
