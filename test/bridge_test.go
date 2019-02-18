package test

import (
	"testing"
)

// helper to close both conns
func closeBridge(br *Bridge) {
	br.conn0.Close()
	br.conn1.Close()
}

type AsyncResult struct {
	n   int
	err error
}

func TestBridge(t *testing.T) {
	var n int
	var err error

	buf := make([]byte, 256)

	t.Run("normal", func(t *testing.T) {
		readRes := make(chan AsyncResult)
		msg := "ABC"
		br := NewBridge()
		conn0 := br.GetConn0()
		conn1 := br.GetConn1()

		n, err = conn0.Write([]byte(msg))
		if err != nil {
			t.Error(err.Error())
		}
		if n != len(msg) {
			t.Error("unexpected length")
		}

		go func() {
			n, err := conn1.Read(buf)
			readRes <- AsyncResult{n: n, err: err}
		}()

		br.Process()

		ar := <-readRes
		if ar.err != nil {
			t.Error(err.Error())
		}
		if ar.n != len(msg) {
			t.Error("unexpected length")
		}
		closeBridge(br)
	})

	t.Run("drop 1st packet from conn0", func(t *testing.T) {
		readRes := make(chan AsyncResult)
		msg1 := "ABC"
		msg2 := "DEFG"
		br := NewBridge()
		conn0 := br.GetConn0()
		conn1 := br.GetConn1()

		n, err = conn0.Write([]byte(msg1))
		if err != nil {
			t.Error(err.Error())
		}
		if n != len(msg1) {
			t.Error("unexpected length")
		}
		n, err = conn0.Write([]byte(msg2))
		if err != nil {
			t.Error(err.Error())
		}
		if n != len(msg2) {
			t.Error("unexpected length")
		}

		go func() {
			n, err := conn1.Read(buf)
			readRes <- AsyncResult{n: n, err: err}
		}()

		br.Drop(0, 0, 1)
		br.Process()

		ar := <-readRes
		if ar.err != nil {
			t.Error(err.Error())
		}
		if ar.n != len(msg2) {
			t.Error("unexpected length")
		}
		closeBridge(br)
	})

	t.Run("drop 2nd packet from conn0", func(t *testing.T) {
		readRes := make(chan AsyncResult)
		msg1 := "ABC"
		msg2 := "DEFG"
		br := NewBridge()
		conn0 := br.GetConn0()
		conn1 := br.GetConn1()

		n, err = conn0.Write([]byte(msg1))
		if err != nil {
			t.Error(err.Error())
		}
		if n != len(msg1) {
			t.Error("unexpected length")
		}
		n, err = conn0.Write([]byte(msg2))
		if err != nil {
			t.Error(err.Error())
		}
		if n != len(msg2) {
			t.Error("unexpected length")
		}

		go func() {
			n, err := conn1.Read(buf)
			readRes <- AsyncResult{n: n, err: err}
		}()

		br.Drop(0, 1, 1)
		br.Process()

		ar := <-readRes
		if ar.err != nil {
			t.Error(err.Error())
		}
		if ar.n != len(msg1) {
			t.Error("unexpected length")
		}
		closeBridge(br)
	})

	t.Run("drop 1st packet from conn1", func(t *testing.T) {
		readRes := make(chan AsyncResult)
		msg1 := "ABC"
		msg2 := "DEFG"
		br := NewBridge()
		conn0 := br.GetConn0()
		conn1 := br.GetConn1()

		n, err = conn1.Write([]byte(msg1))
		if err != nil {
			t.Error(err.Error())
		}
		if n != len(msg1) {
			t.Error("unexpected length")
		}
		n, err = conn1.Write([]byte(msg2))
		if err != nil {
			t.Error(err.Error())
		}
		if n != len(msg2) {
			t.Error("unexpected length")
		}

		go func() {
			n, err := conn0.Read(buf)
			readRes <- AsyncResult{n: n, err: err}
		}()

		br.Drop(1, 0, 1)
		br.Process()

		ar := <-readRes
		if ar.err != nil {
			t.Error(err.Error())
		}
		if ar.n != len(msg2) {
			t.Error("unexpected length")
		}
		closeBridge(br)
	})

	t.Run("drop 2nd packet from conn1", func(t *testing.T) {
		readRes := make(chan AsyncResult)
		msg1 := "ABC"
		msg2 := "DEFG"
		br := NewBridge()
		conn0 := br.GetConn0()
		conn1 := br.GetConn1()

		n, err = conn1.Write([]byte(msg1))
		if err != nil {
			t.Error(err.Error())
		}
		if n != len(msg1) {
			t.Error("unexpected length")
		}
		n, err = conn1.Write([]byte(msg2))
		if err != nil {
			t.Error(err.Error())
		}
		if n != len(msg2) {
			t.Error("unexpected length")
		}

		go func() {
			n, err := conn0.Read(buf)
			readRes <- AsyncResult{n: n, err: err}
		}()

		br.Drop(1, 1, 1)
		br.Process()

		ar := <-readRes
		if ar.err != nil {
			t.Error(err.Error())
		}
		if ar.n != len(msg1) {
			t.Error("unexpected length")
		}
		closeBridge(br)
	})

	t.Run("reorder packets from conn0", func(t *testing.T) {
		msg1 := "ABC"
		msg2 := "DEFG"
		br := NewBridge()
		conn0 := br.GetConn0()
		conn1 := br.GetConn1()

		n, err = conn0.Write([]byte(msg1))
		if err != nil {
			t.Error(err.Error())
		}
		if n != len(msg1) {
			t.Error("unexpected length")
		}
		n, err = conn0.Write([]byte(msg2))
		if err != nil {
			t.Error(err.Error())
		}
		if n != len(msg2) {
			t.Error("unexpected length")
		}

		done := make(chan bool)

		go func() {
			n, err := conn1.Read(buf)
			if err != nil {
				t.Error(err.Error())
			}
			if n != len(msg2) {
				t.Error("unexpected length")
			}
			n, err = conn1.Read(buf)
			if err != nil {
				t.Error(err.Error())
			}
			if n != len(msg1) {
				t.Error("unexpected length")
			}
			done <- true
		}()

		err = br.Reorder(0)
		if err != nil {
			t.Error(err.Error())
		}
		br.Process()
		<-done
		closeBridge(br)
	})

	t.Run("reorder packets from conn1", func(t *testing.T) {
		msg1 := "ABC"
		msg2 := "DEFG"
		br := NewBridge()
		conn0 := br.GetConn0()
		conn1 := br.GetConn1()

		n, err = conn1.Write([]byte(msg1))
		if err != nil {
			t.Error(err.Error())
		}
		if n != len(msg1) {
			t.Error("unexpected length")
		}
		n, err = conn1.Write([]byte(msg2))
		if err != nil {
			t.Error(err.Error())
		}
		if n != len(msg2) {
			t.Error("unexpected length")
		}

		done := make(chan bool)

		go func() {
			n, err := conn0.Read(buf)
			if err != nil {
				t.Error(err.Error())
			}
			if n != len(msg2) {
				t.Error("unexpected length")
			}
			n, err = conn0.Read(buf)
			if err != nil {
				t.Error(err.Error())
			}
			if n != len(msg1) {
				t.Error("unexpected length")
			}
			done <- true
		}()

		err = br.Reorder(1)
		if err != nil {
			t.Error(err.Error())
		}
		br.Process()
		<-done
		closeBridge(br)
	})

	t.Run("inverse error", func(t *testing.T) {
		q := [][]byte{}
		q = append(q, []byte("ABC"))
		err := inverse(q)
		if err == nil {
			t.Error("inverse should fail if less than 2 pkts")
		}
	})

	t.Run("read closed conn", func(t *testing.T) {
		br := NewBridge()
		conn0 := br.GetConn0()
		conn1 := br.GetConn1()

		conn0.Close()
		conn1.Close()

		_, err = conn0.Read(buf)
		if err == nil {
			t.Error("read should fail as conn is closed")
		}
	})
}
