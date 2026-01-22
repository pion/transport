// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !wasm
// +build !wasm

package test

func filterRoutineWASM(string) bool {
	return false
}
