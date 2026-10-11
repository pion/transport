// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build linux || (darwin && !ios)

package netchange

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"

	"golang.org/x/sys/unix"
)

type socket struct {
	file   *os.File
	filter func([]byte) bool
}

func (s *socket) drain() (bool, error) {
	defer runtime.KeepAlive(s.file)
	fd := int(s.file.Fd()) //nolint:gosec
	var buf [4]byte
	notified := false
	for range 64 {
		n, err := unix.Read(fd, buf[:])
		switch {
		case errors.Is(err, unix.EAGAIN):
			return notified, nil
		case errors.Is(err, unix.ENOBUFS):
			notified = true
		case errors.Is(err, unix.EINTR):
			continue
		case err != nil:
			return notified, fmt.Errorf("drain notification socket: %w", err)
		default:
			notified = notified || s.accepts(buf[:n])
		}
	}

	return notified, nil
}

func (s *socket) close() error {
	return s.file.Close()
}

func (s *socket) wait(ctx context.Context) error {
	if err := context.Cause(ctx); err != nil {
		return err
	}
	if err := s.file.SetReadDeadline(time.Time{}); err != nil {
		return err
	}
	done := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		_ = s.file.SetReadDeadline(time.Now())
		close(done)
	})
	defer func() {
		if !stop() {
			<-done
		}
	}()
	var buf [4]byte
	for {
		n, err := s.file.Read(buf[:])
		if cause := context.Cause(ctx); cause != nil {
			return cause
		}
		if errors.Is(err, unix.ENOBUFS) || errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil || s.accepts(buf[:n]) {
			return err
		}
	}
}

func (s *socket) accepts(message []byte) bool {
	return s.filter == nil || s.filter(message)
}
