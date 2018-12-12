package test

import (
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"strings"
	"testing"
	"time"
)

// TimeOut is used to panic if a test takes to long.
// It will print the current goroutines and panic.
// It is meant as an aid in debugging deadlocks.
func TimeOut(t time.Duration) *time.Timer {
	return time.AfterFunc(t, func() {
		if err := pprof.Lookup("goroutine").WriteTo(os.Stdout, 1); err != nil {
			fmt.Printf("failed to print goroutines: %v \n", err)
		}
		panic("timeout")
	})
}

// CheckRoutines is used to check for leaked go-routines
func CheckRoutines(t *testing.T) (report func()) {
	expectedGoRoutineCount := runtime.NumGoroutine()
	return func() {
		// Wait a little for routines to die
		// TODO: is there a better way?
		time.Sleep(time.Millisecond * 100)

		goRoutineCount := runtime.NumGoroutine()
		if goRoutineCount != expectedGoRoutineCount {
			if err := pprof.Lookup("goroutine").WriteTo(os.Stderr, 1); err != nil {
				t.Fatal(err)
			}
			t.Fatalf("goRoutineCount != expectedGoRoutineCount, possible leak: %d %d", goRoutineCount, expectedGoRoutineCount)
		}
	}
}

// GatherErrs gathers all errors returned by a channel.
// It blocks until the channel is closed.
func GatherErrs(c chan error) []error {
	var errs []error

	for err := range c {
		errs = append(errs, err)
	}

	return errs
}

// FlattenErrs flattens a slice of errors into a single error
func FlattenErrs(errs []error) error {
	var errstrings []string

	for _, err := range errs {
		if err != nil {
			errstrings = append(errstrings, err.Error())
		}
	}

	if len(errstrings) == 0 {
		return nil
	}

	return fmt.Errorf(strings.Join(errstrings, "\n"))
}
