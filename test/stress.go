package test

import (
	"bytes"
	"fmt"
	"io"
	"sync"
)

// Options represents the configuration of the stress test
type Options struct {
	MsgSize  int
	MsgCount int
}

// Stress enables stress testing of a io.ReadWriter.
// It checks that packets are received correctly and in order.
func Stress(ca io.Writer, cb io.Reader, opt Options) error {
	bufs := make(chan []byte, opt.MsgCount)
	errCh := make(chan error)
	// Write
	go func() {
		err := write(ca, bufs, opt)
		errCh <- err
		close(bufs)
	}()

	// Read
	go func() {
		result := make([]byte, opt.MsgSize)

		for original := range bufs {
			err := read(cb, original, result)
			if err != nil {
				errCh <- err
			}
		}

		close(errCh)
	}()

	return FlattenErrs(GatherErrs(errCh))
}

func read(r io.Reader, original, result []byte) error {
	n, err := r.Read(result)
	if err != nil {
		return err
	}
	if !bytes.Equal(original, result[:n]) {
		return fmt.Errorf("byte sequence changed %#v != %#v", original, result)
	}

	return nil
}

// StressDuplex enables duplex stress testing of a io.ReadWriter.
// It checks that packets are received correctly and in order.
func StressDuplex(ca io.ReadWriter, cb io.ReadWriter, opt Options) error {
	errCh := make(chan error)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		errCh <- Stress(ca, cb, opt)

	}()

	go func() {
		defer wg.Done()
		errCh <- Stress(cb, ca, opt)

	}()

	go func() {
		wg.Wait()
		close(errCh)
	}()

	return FlattenErrs(GatherErrs(errCh))
}

func write(c io.Writer, bufs chan []byte, opt Options) error {
	for i := 0; i < opt.MsgCount; i++ {
		buf, err := randBuf(opt.MsgSize)
		if err != nil {
			return err
		}
		bufs <- buf
		if _, err = c.Write(buf); err != nil {
			return err
		}
	}
	return nil
}
