// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package vnet

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewDelayFilterOptions(t *testing.T) {
	t.Run("invalid delay", func(t *testing.T) {
		nic := newMockNIC(t)
		_, err := NewDelayFilter(nic, WithDelay(-time.Millisecond))
		assert.ErrorIs(t, err, ErrInvalidDelay)
	})

	t.Run("nil option ignored", func(t *testing.T) {
		nic := newMockNIC(t)
		filter, err := NewDelayFilter(nic, nil, WithDelay(0))
		assert.NoError(t, err)
		assert.NoError(t, filter.Close())
	})

	t.Run("default delay zero", func(t *testing.T) {
		nic := newMockNIC(t)
		filter, err := NewDelayFilter(nic)
		assert.NoError(t, err)
		assert.NoError(t, filter.Close())
	})
}
