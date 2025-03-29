// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCheckRoutines(t *testing.T) {
	// Limit runtime in case of deadlocks
	lim := TimeOut(time.Second * 20)
	defer lim.Stop()

	// Check for leaking routines
	report := CheckRoutines(t)
	defer report()

	go func() {
		time.Sleep(1 * time.Second)
	}()
}

func TestCheckRoutinesStrict(t *testing.T) {
	mock := &tbMock{TB: t}

	// Limit runtime in case of deadlocks
	lim := TimeOut(time.Second * 20)
	defer lim.Stop()

	// Check for leaking routines
	report := CheckRoutinesStrict(mock)
	defer func() {
		report()
		assert.NotEmpty(t, mock.fatalfCalled, "expected Fatalf to be called")
		assert.Contains(t, mock.fatalfCalled[0], "Unexpected routines")
	}()

	go func() {
		time.Sleep(1 * time.Second)
	}()
}

type tbMock struct {
	testing.TB

	fatalfCalled []string
}

func (m *tbMock) Fatalf(format string, args ...any) {
	m.fatalfCalled = append(m.fatalfCalled, fmt.Sprintf(format, args...))
}
