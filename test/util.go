// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package test

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"strings"
	"testing"
	"time"
)

var errFlattenErrs = errors.New("")

// TimeOut is used to panic if a test takes to long.
// It will print the current goroutines and panic.
// It is meant as an aid in debugging deadlocks.
func TimeOut(t time.Duration) *time.Timer {
	return time.AfterFunc(t, func() {
		if err := pprof.Lookup("goroutine").WriteTo(os.Stdout, 1); err != nil {
			fmt.Printf("failed to print goroutines: %v \n", err) // nolint
		}
		panic("timeout") // nolint
	})
}

func tryCheckRoutinesLoop(tb testing.TB, failMessage string) {
	try := 0
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		runtime.GC()
		routines := getRoutines()
		if len(routines) == 0 {
			return
		}
		if try >= 50 {
			tb.Fatalf("%s: \n%s", failMessage, strings.Join(routines, "\n\n")) // nolint
		}
		try++
	}
}

// CheckRoutines is used to check for leaked go-routines
func CheckRoutines(t *testing.T) func() {
	tryCheckRoutinesLoop(t, "Unexpected routines on test startup")
	return func() {
		tryCheckRoutinesLoop(t, "Unexpected routines on test end")
	}
}

// CheckRoutinesStrict is used to check for leaked go-routines.
// It differs from CheckRoutines in that it has very little tolerance
// for lingering goroutines. This is helpful for tests that need
// to ensure clean closure of resources.
// Checking the state of goroutines exactly is tricky. As users writing
// goroutines, we tend to clean up gracefully using some synchronization
// pattern. When used correctly, we won't leak goroutines, but we cannot
// guarantee *when* the goroutines will end. This is the nature of waiting
// on the runtime's goexit1 being called which is the final subroutine
// called, which is after any user written code. This small, but possible
// chance to have a thread (not goroutine) be preempted before this is
// called, can have our goroutine stack be not quite correct yet. The
// best we can do is sleep a little bit and try to encourage the runtime
// to run that goroutine (G) on the machine (M) it belongs to.
func CheckRoutinesStrict(tb testing.TB) func() {
	tryCheckRoutinesLoop(tb, "Unexpected routines on test startup")
	return func() {
		runtime.Gosched()
		runtime.GC()
		routines := getRoutines()
		if len(routines) == 0 {
			return
		}
		// arbitrarily short choice to allow the runtime to cleanup any
		// goroutines that really aren't doing anything but haven't yet
		// completed.
		time.Sleep(time.Millisecond)
		runtime.Gosched()
		runtime.GC()
		routines = getRoutines()
		if len(routines) == 0 {
			return
		}

		tb.Fatalf("%s: \n%s", "Unexpected routines on test end", strings.Join(routines, "\n\n")) // nolint
	}
}

func getRoutines() []string {
	buf := make([]byte, 2<<20)
	buf = buf[:runtime.Stack(buf, true)]
	return filterRoutines(strings.Split(string(buf), "\n\n"))
}

func filterRoutines(routines []string) []string {
	result := []string{}
	for _, stack := range routines {
		if stack == "" || // Empty
			filterRoutineWASM(stack) || // WASM specific exception
			strings.Contains(stack, "testing.Main(") || // Tests
			strings.Contains(stack, "testing.(*T).Run(") || // Test run
			strings.Contains(stack, "test.getRoutines(") { // This routine
			continue
		}
		result = append(result, stack)
	}
	return result
}

// GatherErrs gathers all errors returned by a channel.
// It blocks until the channel is closed.
func GatherErrs(c chan error) []error {
	errs := make([]error, 0)

	for err := range c {
		errs = append(errs, err)
	}

	return errs
}

// FlattenErrs flattens a slice of errors into a single error
func FlattenErrs(errs []error) error {
	var errStrings []string

	for _, err := range errs {
		if err != nil {
			errStrings = append(errStrings, err.Error())
		}
	}

	if len(errStrings) == 0 {
		return nil
	}

	return fmt.Errorf("%w %s", errFlattenErrs, strings.Join(errStrings, "\n"))
}
