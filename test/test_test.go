package test

import (
	"io"
	"net"
	"testing"
)

func TestStressIOPipe(t *testing.T) {
	r, w := io.Pipe()

	opt := Options{
		MsgSize:  2048,
		MsgCount: 100,
	}

	err := Stress(w, r, opt)
	if err != nil {
		t.Fatal(err)
	}
}

func TestStressDuplexNetPipe(t *testing.T) {
	ca, cb := net.Pipe()

	opt := Options{
		MsgSize:  2048,
		MsgCount: 100,
	}

	err := StressDuplex(ca, cb, opt)
	if err != nil {
		t.Fatal(err)
	}
}

func BenchmarkPipe(b *testing.B) {
	ca, cb := net.Pipe()

	b.ResetTimer()

	opt := Options{
		MsgSize:  2048,
		MsgCount: b.N,
	}

	check(Stress(ca, cb, opt))
}

func BenchmarkUDP(b *testing.B) {
	var ca net.Conn
	var cb net.Conn

	ca, err := net.ListenUDP(udpString, nil)
	check(err)
	defer func() {
		check(ca.Close())
	}()

	cb, err = net.Dial(udpString, ca.LocalAddr().String())
	check(err)
	defer func() {
		check(cb.Close())
	}()

	b.ResetTimer()

	opt := Options{
		MsgSize:  2048,
		MsgCount: b.N,
	}

	check(Stress(cb, ca, opt))
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}
