package transport

import (
	"bytes"
	"io"
	"net"
	"sync"
	"testing"
)

// Test writing and reading from a wrapped connection with the batch methods.
func TestNewConn(t *testing.T) {
	r, w := net.Pipe()

	// NOTE: Both of these will use messageSingler.
	rc := NewConn(r)
	wc := NewConn(w)

	wg := sync.WaitGroup{}
	wg.Add(2)

	messageCount := 10
	messageContents := func(n int) (message []byte) {
		// This function can produce anything, so long as it's <= 10 bytes.
		for i := 0; i < n+1; i++ {
			message = append(message, byte(n+i))
		}

		return message
	}

	go func() {
		defer wg.Done()

		count := 0

		for {
			// Read at most three messages at a time.
			ms := make([]Message, 3)

			for i := range ms {
				// Read at most 10 bytes at a time.
				ms[i].Buffer = make([]byte, 10)
			}

			n, err := rc.ReadBatch(ms)
			if err == io.EOF {
				break
			} else if err != nil {
				t.Error(err)
			}

			if n <= 0 {
				t.Error("number read in non-positive")
			}

			for i := 0; i < n; i++ {
				m := ms[i]
				expected := messageContents(count + i)

				if len(expected) != m.Size {
					t.Error("wrong message size")
				}

				if !bytes.Equal(expected, m.Buffer[:m.Size]) {
					t.Error("wrong message contents")
				}
			}

			count += n
		}

		if count != messageCount {
			t.Error("wrong number of messages received")
		}

		err := rc.Close()
		if err != nil {
			t.Error(err)
		}
	}()

	go func() {
		defer wg.Done()

		count := 0

		for count < messageCount {
			// Write at most four messages at a time.
			num := 4

			// Send fewer than four messages if we go over.
			if num > messageCount-count {
				num = messageCount - count
			}

			ms := make([]Message, num)

			// Populate our messages with the contents.
			for i := range ms {
				ms[i].Buffer = messageContents(count + i)
			}

			// Try to write the batch of messages.
			n, err := wc.WriteBatch(ms)
			if err != nil {
				t.Error(err)
			}

			if n <= 0 {
				t.Error("number written in non-positive")
			}

			for i := 0; i < n; i++ {
				m := ms[i]

				if m.Size != len(messageContents(count+i)) {
					t.Error("wrong number of written bytes")
				}
			}

			count += n
		}

		if count != messageCount {
			t.Error("wrong number of messages sent")
		}

		err := wc.Close()
		if err != nil {
			t.Error(err)
		}
	}()

	wg.Wait()
}
