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
func CheckRoutines(t *testing.T) func() {
	initial := getRoutines()
	return func() {
		try := 0
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			routines := getRoutines()
			if len(routines) <= len(initial) {
				return
			}
			if try >= 50 {
				t.Fatalf("Unexpected routines: \n%s", strings.Join(routines, "\n\n"))
			}
			try++
		}
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
