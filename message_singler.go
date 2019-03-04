package transport

import (
	"net"
)

type messageSingler struct {
	net.Conn
}

func newMessageSingler(conn net.Conn) (ms *messageSingler) {
	return &messageSingler{conn}
}

func (msr *messageSingler) ReadBatch(ms []Message) (n int, err error) {
	if len(ms) == 0 {
		return 0, nil
	}

	// We can only read a single message with blocking IO.
	// It may be worth implementing non-blocking IO to slightly improve performance.

	// The pointer is required to modify the slice value.
	m := &ms[0]

	m.Size, err = msr.Read(m.Buffer)
	if err != nil {
		return 0, err
	}

	return 1, nil
}

func (msr *messageSingler) WriteBatch(ms []Message) (n int, err error) {
	// We could just write a single message, but loop for better performance.
	// This will avoid function overhead and having the caller potentially reconstruct the Message slice.

	for i := range ms {
		// The pointer is required to modify the slice value.
		m := &ms[i]

		m.Size, err = msr.Write(m.Buffer)
		if err != nil {
			// Return the number of successful messages on error.
			return i, err
		}
	}

	return len(ms), nil
}
