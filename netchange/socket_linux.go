// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build linux

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
	file *os.File
}

func openSocket() (notificationSource, error) {
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC, unix.NETLINK_ROUTE)
	if err != nil {
		return nil, fmt.Errorf("open netlink socket: %w", err)
	}
	address := &unix.SockaddrNetlink{
		Family: unix.AF_NETLINK,
		Groups: unix.RTMGRP_LINK | unix.RTMGRP_IPV4_IFADDR | unix.RTMGRP_IPV6_IFADDR,
	}
	if err = unix.Bind(fd, address); err != nil {
		_ = unix.Close(fd)

		return nil, fmt.Errorf("subscribe to netlink notifications: %w", err)
	}

	return &socket{file: os.NewFile(uintptr(fd), "netlink")}, nil //nolint:gosec // fd is nonnegative.
}

func (s *socket) drain() (bool, error) {
	defer runtime.KeepAlive(s.file)
	fd := int(s.file.Fd()) //nolint:gosec
	var buf [1]byte
	notified := false
	for range 64 {
		_, _, err := unix.Recvfrom(fd, buf[:], unix.MSG_DONTWAIT)
		switch {
		case errors.Is(err, unix.EAGAIN):
			return notified, nil
		case errors.Is(err, unix.ENOBUFS):
			notified = true
		case errors.Is(err, unix.EINTR):
			continue
		case err != nil:
			return notified, fmt.Errorf("drain netlink socket: %w", err)
		default:
			notified = true
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
	var buf [1]byte
	_, err := s.file.Read(buf[:])
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	if errors.Is(err, unix.ENOBUFS) || errors.Is(err, io.EOF) {
		return nil
	}

	return err
}
