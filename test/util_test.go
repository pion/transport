// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package test

import (
	"fmt"
	"strings"
	"testing"
	"time"
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
		if len(mock.fatalfCalled) == 0 {
			t.Error("expected Fatalf to be called")
		}
		if !strings.Contains(mock.fatalfCalled[0], "Unexpected routines") {
			t.Error("expected 'Unexpected routines'")
		}
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
