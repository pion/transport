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

	m := &ms[0]

	m.Size, err = msr.Read(m.Buffer)
	if err != nil {
		return 0, err
	}

	return 1, nil
}

func (msr *messageSingler) WriteBatch(ms []Message) (n int, err error) {
	if len(ms) == 0 {
		return 0, nil
	}

	m := &ms[0]

	m.Size, err = msr.Write(m.Buffer)
	if err != nil {
		return 0, err
	}

	return 1, nil
}
