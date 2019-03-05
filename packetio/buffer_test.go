package packetio

import (
	"io"
	"testing"
	"time"

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
	buffer.SetLimitSize(5)

	assert.Equal(0, buffer.Size())

	// Write twice
	n, err := buffer.Write([]byte{0, 1})
	assert.NoError(err)
	assert.Equal(2, n)
	assert.Equal(2, buffer.Size())

	n, err = buffer.Write([]byte{2, 3})
	assert.NoError(err)
	assert.Equal(2, n)
	assert.Equal(4, buffer.Size())

	// Over capacity
	_, err = buffer.Write([]byte{4, 5})
	assert.Equal(ErrFull, err)
	assert.Equal(4, buffer.Size())

	// Cheeky write at exact size.
	n, err = buffer.Write([]byte{6})
	assert.NoError(err)
	assert.Equal(1, n)
	assert.Equal(5, buffer.Size())

	// Read once
	packet := make([]byte, 4)
	n, err = buffer.Read(packet)
	assert.NoError(err)
	assert.Equal(2, n)
	assert.Equal([]byte{0, 1}, packet[:n])
	assert.Equal(3, buffer.Size())

	// Write once
	n, err = buffer.Write([]byte{7, 8})
	assert.NoError(err)
	assert.Equal(2, n)
	assert.Equal(5, buffer.Size())

	// Over capacity
	_, err = buffer.Write([]byte{9, 10})
	assert.Equal(ErrFull, err)
	assert.Equal(5, buffer.Size())

	// Read everything
	n, err = buffer.Read(packet)
	assert.NoError(err)
	assert.Equal(2, n)
	assert.Equal([]byte{2, 3}, packet[:n])
	assert.Equal(3, buffer.Size())

	n, err = buffer.Read(packet)
	assert.NoError(err)
	assert.Equal(1, n)
	assert.Equal([]byte{6}, packet[:n])
	assert.Equal(2, buffer.Size())

	n, err = buffer.Read(packet)
	assert.NoError(err)
	assert.Equal(2, n)
	assert.Equal([]byte{7, 8}, packet[:n])
	assert.Equal(0, buffer.Size())

	// Nothing left.
	err = buffer.Close()
	assert.NoError(err)
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

	// Try again with the right size
	packet = make([]byte, 4)
	n, err = buffer.Read(packet)
	assert.NoError(err)
	assert.Equal(4, n)

	// Close
	err = buffer.Close()
	assert.NoError(err)

	// Make sure you can Close twice
	err = buffer.Close()
	assert.NoError(err)
}

func benchmarkBuffer(b *testing.B, size int64) {
	buffer := NewBuffer()
	b.SetBytes(size)

	done := make(chan struct{})
	go func() {
		packet := make([]byte, size)

		for {
			_, err := buffer.Read(packet)
			if err == io.EOF {
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
		_, err := buffer.Write(packet)
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
