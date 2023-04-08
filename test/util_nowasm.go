//go:build !wasm
// +build !wasm

package test

func filterRoutineWASM(string) bool {
	return false
}
