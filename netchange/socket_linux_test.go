// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build linux

package netchange

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestDrainDatagrams(t *testing.T) {
	sender, receiver := newSocketPair(t)
	for _, size := range []int{0, 1, 64, 64 * 1024} {
		assert.NoError(t, unix.Send(sender, make([]byte, size), 0))
	}
	notified, err := receiver.drain()
	assert.NoError(t, err)
	assert.True(t, notified)
	notified, err = receiver.drain()
	assert.NoError(t, err)
	assert.False(t, notified, "all datagrams were consumed, including oversized payloads")
}

func TestWaitSocket(t *testing.T) {
	sender, receiver := newSocketPair(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.ErrorIs(t, receiver.wait(ctx), context.Canceled)

	ctx, cancel = context.WithTimeoutCause(context.Background(), 20*time.Millisecond, io.ErrNoProgress)
	assert.ErrorIs(t, receiver.wait(ctx), io.ErrNoProgress)
	cancel()

	// A canceled wait must not poison the next wait on the same descriptor.
	state := []interfaceState{{Index: 1, Name: "before"}}
	detector := &Detector{
		source:    receiver,
		enumerate: func() ([]interfaceState, error) { return state, nil },
	}
	assert.NoError(t, detector.start())
	_, err := detector.Check(context.Background())
	assert.NoError(t, err)
	assert.NoError(t, unix.Send(sender, []byte{1}, 0))
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	assert.NoError(t, detector.wait(ctx))
	notified, err := receiver.drain()
	assert.NoError(t, err)
	assert.False(t, notified, "waiting consumes the notification")
	state[0].Name = "after" //nolint:goconst
	changes, err := detector.Check(ctx)
	assert.NoError(t, err)
	assert.Equal(t, []Change{{Interface: "after", Type: Changed}}, changes)
}

func TestDetectorSystemBackend(t *testing.T) {
	detector, err := NewDetector(nil, WithInterfaceFilter(func(string) bool { return false }))
	if !assert.NoError(t, err) {
		return
	}
	t.Cleanup(func() { _ = detector.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = detector.Check(ctx)
	assert.ErrorIs(t, err, context.Canceled)

	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	changes, err := detector.Check(ctx)
	assert.NoError(t, err)
	assert.Empty(t, changes, "the first call returns immediately when all interfaces are filtered out")
	_, err = detector.Check(ctx)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.NoError(t, detector.Close())
	_, err = detector.Check(context.Background())
	assert.ErrorIs(t, err, os.ErrClosed)
	assert.ErrorIs(t, detector.Close(), os.ErrClosed)
}

func TestPlatformTimeout(t *testing.T) {
	sender, receiver := newSocketPair(t)
	name := "before"
	refreshes := 0
	detector := &Detector{
		source: receiver,
		enumerate: func() ([]interfaceState, error) {
			refreshes++
			if refreshes == 3 {
				name = "after"
			}

			return []interfaceState{{Index: 1, Name: name}}, nil
		},
	}
	WithPlatformTimeout(5 * time.Millisecond)(detector)
	require.NoError(t, detector.start())
	_, err := detector.Check(context.Background())
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	changes, err := detector.Check(ctx)
	require.NoError(t, err)
	require.Equal(t, []Change{{Interface: "after", Type: Changed}}, changes)
	require.Equal(t, 3, refreshes, "unchanged timeout refreshes must keep waiting")

	name = "notified"
	require.NoError(t, unix.Send(sender, []byte{1}, 0))
	changes, err = detector.Check(ctx)
	require.NoError(t, err)
	require.Equal(t, []Change{{Interface: "notified", Type: Changed}}, changes)

	ctx, cancelCause := context.WithTimeoutCause(context.Background(), 20*time.Millisecond, io.ErrNoProgress)
	defer cancelCause()
	changes, err = detector.Check(ctx)
	require.ErrorIs(t, err, io.ErrNoProgress, "parent cancellation must not be swallowed")
	require.Empty(t, changes)
}

func newSocketPair(t *testing.T) (int, *socket) {
	t.Helper()
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_DGRAM|unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC, 0)
	assert.NoError(t, err)
	receiver := &socket{file: os.NewFile(uintptr(fds[1]), "netlink-test")}
	t.Cleanup(func() {
		assert.NoError(t, unix.Close(fds[0]))
		assert.NoError(t, receiver.close())
	})

	return fds[0], receiver
}
