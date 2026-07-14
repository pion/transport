// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package vnet

type timeoutError struct {
	msg string
}

var errIOTimeout = &timeoutError{msg: "i/o timeout"} //nolint:gochecknoglobals

func (e *timeoutError) Error() string {
	return e.msg
}

func (e *timeoutError) Timeout() bool {
	return true
}
