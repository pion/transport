// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package packetio

import (
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBufferWraparound(t *testing.T) {
	for _, offset := range []int{11, 13} {
		for _, grow := range []bool{false, true} {
			t.Run(fmt.Sprintf("offset=%d/grow=%t", offset, grow), func(t *testing.T) {
				buffer := NewBuffer()
				assert.NoError(t, buffer.grow())
				buffer.head = len(buffer.data) - offset
				buffer.tail = buffer.head
				write := func(value byte) {
					t.Helper()
					_, err := buffer.Write([]byte{value, value, value}, Attributes{{Key: testKey, Value: value}})
					assert.NoError(t, err)
				}
				read := func(value byte) {
					t.Helper()
					payload := make([]byte, 3)
					n, attributes, err := buffer.Read(payload, nil)
					assert.NoError(t, err)
					assert.Equal(t, []byte{value, value, value}, payload[:n])
					assert.Equal(t, value, attributes.Get(testKey))
				}
				for value := range byte(4) {
					write(value)
				}
				read(0)
				read(1)
				write(4)
				write(5) // Wrap the attribute ring.
				write(6) // Grow the wrapped attribute ring.
				if grow {
					assert.NoError(t, buffer.grow()) // Grow the wrapped byte ring.
				}
				for value := byte(2); value <= 6; value++ {
					read(value)
				}
				assert.Zero(t, buffer.Count())
				assert.Zero(t, buffer.Size())
				for _, attributes := range buffer.attributes {
					assert.Empty(t, attributes)
				}
			})
		}
	}
}

func TestBufferLimitSize(t *testing.T) {
	assert := assert.New(t)

	buffer := NewBuffer()
	buffer.SetLimitSize(11)

	assert.Equal(0, buffer.Size())

	// Write twice
	n, err := buffer.Write([]byte{0, 1}, nil)
	assert.NoError(err)
	assert.Equal(2, n)
	assert.Equal(4, buffer.Size())

	n, err = buffer.Write([]byte{2, 3}, nil)
	assert.NoError(err)
	assert.Equal(2, n)
	assert.Equal(8, buffer.Size())

	// Over capacity
	_, err = buffer.Write([]byte{4, 5}, nil)
	assert.Equal(ErrFull, err)
	assert.Equal(8, buffer.Size())

	// Cheeky write at exact size.
	n, err = buffer.Write([]byte{6}, nil)
	assert.NoError(err)
	assert.Equal(1, n)
	assert.Equal(11, buffer.Size())

	// Read once
	packet := make([]byte, 4)
	n, _, err = buffer.Read(packet, nil)
	assert.NoError(err)
	assert.Equal(2, n)
	assert.Equal([]byte{0, 1}, packet[:n])
	assert.Equal(7, buffer.Size())

	// Write once
	n, err = buffer.Write([]byte{7, 8}, nil)
	assert.NoError(err)
	assert.Equal(2, n)
	assert.Equal(11, buffer.Size())

	// Over capacity
	_, err = buffer.Write([]byte{9, 10}, nil)
	assert.Equal(ErrFull, err)
	assert.Equal(11, buffer.Size())

	// Read everything
	n, _, err = buffer.Read(packet, nil)
	assert.NoError(err)
	assert.Equal(2, n)
	assert.Equal([]byte{2, 3}, packet[:n])
	assert.Equal(7, buffer.Size())

	n, _, err = buffer.Read(packet, nil)
	assert.NoError(err)
	assert.Equal(1, n)
	assert.Equal([]byte{6}, packet[:n])
	assert.Equal(4, buffer.Size())

	n, _, err = buffer.Read(packet, nil)
	assert.NoError(err)
	assert.Equal(2, n)
	assert.Equal([]byte{7, 8}, packet[:n])
	assert.Equal(0, buffer.Size())

	// Nothing left.
	err = buffer.Close()
	assert.NoError(err)
}

func TestBufferAlloc(t *testing.T) {
	for _, entries := range []int{0, 1, 4} {
		t.Run(fmt.Sprint(entries), func(t *testing.T) {
			buffer := NewBuffer()
			packet := make([]byte, 1024)
			var attributes Attributes
			for i := range entries {
				attributes = append(attributes, Attribute{Key: i, Value: time.Unix(42, 0)})
			}
			var received Attributes
			// AllocsPerRun warms up both queue and receive storage.
			allocs := testing.AllocsPerRun(100, func() {
				if _, err := buffer.Write(packet, attributes); err != nil {
					assert.NoError(t, err)

					return
				}
				var err error
				_, received, err = buffer.Read(packet, received)
				if err != nil {
					assert.NoError(t, err)
				}
			})
			assert.Zero(t, allocs)
			assert.Equal(t, attributes, received)
		})
	}
}

func benchmarkBufferWR(b *testing.B, size int64, write bool, grow int) { // nolint:unparam
	b.Helper()
	buffer := NewBuffer()
	packet := make([]byte, size)

	// Grow the buffer first
	pad := make([]byte, 1022)
	for buffer.Size() < grow {
		_, err := buffer.Write(pad, nil)
		if err != nil {
			b.Fatalf("Write: %v", err)
		}
	}
	for buffer.Size() > 0 {
		_, _, err := buffer.Read(pad, nil)
		if err != nil {
			b.Fatalf("Write: %v", err)
		}
	}

	if write {
		_, err := buffer.Write(packet, nil)
		if err != nil {
			b.Fatalf("Write: %v", err)
		}
	}

	b.SetBytes(size)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := buffer.Write(packet, nil)
		if err != nil {
			b.Fatalf("Write: %v", err)
		}
		_, _, err = buffer.Read(packet, nil)
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
			_, _, err := buffer.Read(packet, nil)
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
			_, err = buffer.Write(packet, nil)
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
	buffer := NewBuffer()
	assert.NoError(t, buffer.SetReadDeadline(time.Now().Add(5*time.Second)))
	errors := make(chan error, 2)
	for range cap(errors) {
		go func() {
			_, _, err := buffer.Read(make([]byte, 4), nil)
			errors <- err
		}()
	}
	assert.NoError(t, buffer.Close())
	for range cap(errors) {
		assert.Equal(t, io.EOF, <-errors)
	}
}

type testAttributeKey int

const testKey testAttributeKey = 0

func TestBufferAttributes(t *testing.T) {
	buffer := NewBuffer()
	reference := &struct{ Source string }{"source"}
	input := Attributes{{Key: testKey, Value: 42}, {Key: "reference", Value: reference}, {Key: "nil", Value: nil}}
	payload := []byte("data")
	for range 2 {
		n, err := buffer.Write(payload, input)
		assert.NoError(t, err)
		assert.Equal(t, len(payload), n)
	}
	clear(input)
	clear(payload)
	_, err := buffer.Write([]byte("next"), nil)
	assert.NoError(t, err)
	assert.Equal(t, 12+3*2, buffer.Size())

	n, attributes, err := buffer.Read(payload, nil)
	assert.NoError(t, err)
	assert.Equal(t, "data", string(payload[:n]))
	assert.Equal(t, Attributes{
		{Key: testKey, Value: 42}, {Key: "reference", Value: reference}, {Key: "nil", Value: nil},
	}, attributes)
	assert.Same(t, reference, attributes.Get("reference"))
	attributes.Set(testKey, 99)
	assert.Equal(t, make(Attributes, len(input)), input)

	n, attributes, err = buffer.Read(payload, attributes)
	assert.NoError(t, err)
	assert.Equal(t, "data", string(payload[:n]))
	assert.Equal(t, 42, attributes.Get(testKey))
	assert.Equal(t, 6, buffer.Size())
	n, attributes, err = buffer.Read(payload, attributes)
	assert.NoError(t, err)
	assert.Equal(t, "next", string(payload[:n]))
	assert.Empty(t, attributes)
	assert.Equal(t, make(Attributes, cap(attributes)), attributes[:cap(attributes)])
	assert.Zero(t, buffer.Size())
	for _, value := range buffer.attributes {
		assert.Empty(t, value)
		for _, entry := range value[:cap(value)] {
			assert.Equal(t, Attribute{}, entry)
		}
	}
}

func TestBufferAttributesDeadlineAndClose(t *testing.T) {
	buffer := NewBuffer()
	_, err := buffer.Write([]byte("data"), Attributes{{Key: testKey, Value: 42}})
	assert.NoError(t, err)
	assert.NoError(t, buffer.SetReadDeadline(time.Now().Add(-time.Second)))
	payload := make([]byte, 4)
	n, attributes, err := buffer.Read(payload, nil)
	assert.ErrorIs(t, err, ErrTimeout)
	var timeout net.Error
	if assert.ErrorAs(t, err, &timeout) {
		assert.True(t, timeout.Timeout())
	}
	assert.Zero(t, n)
	assert.Empty(t, attributes)
	assert.Equal(t, 1, buffer.Count())
	assert.NoError(t, buffer.SetReadDeadline(time.Time{}))
	assert.NoError(t, buffer.Close())
	assert.NoError(t, buffer.Close())
	_, err = buffer.Write(nil, Attributes{{Key: testKey, Value: "rejected"}})
	assert.ErrorIs(t, err, io.ErrClosedPipe)
	n, attributes, err = buffer.Read(payload, nil)
	assert.NoError(t, err)
	assert.Equal(t, "data", string(payload[:n]))
	assert.Equal(t, 42, attributes.Get(testKey))
	n, attributes, err = buffer.Read(payload, nil)
	assert.ErrorIs(t, err, io.EOF)
	assert.Zero(t, n)
	assert.Empty(t, attributes)
	assert.Zero(t, buffer.Count())
}

func TestBufferTruncationPreservesAttributes(t *testing.T) {
	for _, size := range []int{0, 3, 6} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			buffer := NewBuffer()
			_, err := buffer.Write([]byte("packet"), Attributes{{Key: testKey, Value: 42}})
			assert.NoError(t, err)
			_, err = buffer.Write(nil, nil)
			assert.NoError(t, err)
			payload := make([]byte, size)
			n, attributes, err := buffer.Read(payload, nil)
			if size < 6 {
				assert.ErrorIs(t, err, io.ErrShortBuffer)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, "packet"[:size], string(payload[:n]))
			assert.Equal(t, 42, attributes.Get(testKey))
			assert.Equal(t, 1, buffer.Count())
			assert.Equal(t, 2, buffer.Size())
			n, attributes, err = buffer.Read(nil, nil)
			assert.NoError(t, err)
			assert.Zero(t, n)
			assert.Empty(t, attributes)
		})
	}
}

func TestBufferAttributeLimits(t *testing.T) {
	buffer := NewBuffer()
	buffer.SetLimitSize(4)
	buffer.SetLimitCount(1)
	input := Attributes{{Key: testKey, Value: 42}}
	_, err := buffer.Write(nil, input)
	assert.NoError(t, err)
	input.Set("extra", true)
	assert.Equal(t, 2, buffer.Size()) // Attributes do not change logical byte accounting.
	_, err = buffer.Write(nil, nil)
	assert.ErrorIs(t, err, ErrFull)
	assert.Equal(t, 1, buffer.Count())
	buffer.SetLimitCount(0)
	_, err = buffer.Write(nil, input)
	assert.NoError(t, err)
	assert.Equal(t, 4, buffer.Size())
	_, err = buffer.Write(nil, nil)
	assert.ErrorIs(t, err, ErrFull)
	buffer.SetLimitSize(1)
	for _, expected := range []Attributes{{{Key: testKey, Value: 42}}, input} {
		_, attributes, readErr := buffer.Read(nil, nil)
		assert.NoError(t, readErr)
		assert.Equal(t, expected, attributes)
	}
	assert.Zero(t, buffer.Size())
	buffer.SetLimitSize(math.MaxInt)
	_, err = buffer.Write(make([]byte, minSize), nil)
	assert.NoError(t, err)
}

func TestBufferAttributeStorageCap(t *testing.T) {
	for _, limit := range []int{128 * 1024, 0, 2 * maxSize} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			buffer := NewBuffer()
			buffer.SetLimitSize(limit)
			capacity := limit
			if limit == 0 || (sizeHardLimit && limit >= maxSize) {
				capacity = maxSize - 1
			}
			payload := make([]byte, 0x8000)
			attributes := Attributes{{Key: testKey, Value: 42}}
			count := capacity / (len(payload) + 2)
			for range count {
				_, err := buffer.Write(payload, attributes)
				assert.NoError(t, err)
			}
			_, err := buffer.Write(payload, attributes)
			assert.ErrorIs(t, err, ErrFull)
			assert.Equal(t, count, buffer.Count())
			assert.NoError(t, buffer.Close())
			for range count {
				n, got, err := buffer.Read(payload, nil)
				assert.NoError(t, err)
				assert.Equal(t, len(payload), n)
				assert.Equal(t, attributes, got)
			}
			assert.Zero(t, buffer.Size())
		})
	}
}

func TestBufferAttributesConcurrent(t *testing.T) {
	buffer := NewBuffer()
	t.Cleanup(func() { assert.NoError(t, buffer.Close()) })
	assert.NoError(t, buffer.SetReadDeadline(time.Now().Add(5*time.Second)))
	const workers, packets = 4, 100
	results := make(chan [2]byte, workers*packets)
	var group sync.WaitGroup
	for writer := range byte(workers) {
		group.Add(2)
		go func() {
			defer group.Done()
			payload := []byte{writer, 0}
			attributes := Attributes{}
			for index := range byte(packets) {
				payload[1] = index
				attributes.Set(testKey, [2]byte{writer, index})
				_, err := buffer.Write(payload, attributes)
				assert.NoError(t, err)
				clear(attributes)
				attributes = attributes[:0]
			}
		}()
		go func() {
			defer group.Done()
			payload := make([]byte, 2)
			var attributes Attributes
			for range packets {
				n, received, err := buffer.Read(payload, attributes)
				attributes = received
				if !assert.NoError(t, err) || !assert.Equal(t, 2, n) {
					return
				}
				value := [2]byte(payload)
				assert.Equal(t, value, attributes.Get(testKey))
				results <- value
			}
		}()
	}
	group.Wait()
	close(results)
	seen := make(map[[2]byte]bool)
	for value := range results {
		assert.False(t, seen[value])
		seen[value] = true
	}
	assert.Len(t, seen, workers*packets)
	assert.Zero(t, buffer.Count())
}

func BenchmarkBufferAttributes(b *testing.B) {
	for _, attributes := range []Attributes{nil, {{Key: testKey, Value: time.Unix(42, 0)}}} {
		b.Run(fmt.Sprintf("entries=%d", len(attributes)), func(b *testing.B) {
			buffer := NewBuffer()
			payload := make([]byte, 1400)
			var received Attributes
			b.ReportAllocs()
			b.SetBytes(int64(len(payload)))
			b.ResetTimer()
			for range b.N {
				if _, err := buffer.Write(payload, attributes); err != nil {
					b.Fatal(err)
				}
				var err error
				_, received, err = buffer.Read(payload, received)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
