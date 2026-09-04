// Package purity marks the region in which the authority decides. A command's
// transform is a pure function of its snapshot and its own payload: everything
// it needs is computed before the command is applied. Nothing here changes what
// any operation does; it only makes that rule checkable.
package purity

import (
	"runtime"
	"strconv"
	"sync"
)

// The depth is per goroutine: a transform runs on the goroutine that applied
// the command, while other goroutines legitimately read files, seal artifacts
// and run processes at the same moment.
var depth sync.Map // goroutine id -> int

// Impure is called by a guarded operation invoked from inside a transform. It
// is nil in a normal build; a test installs a reporter that fails the test.
var Impure func(operation string)

// Enter marks the start of a transform and returns its end.
func Enter() func() {
	id := goroutineID()
	current, _ := depth.Load(id)
	entered, _ := current.(int)
	depth.Store(id, entered+1)
	return func() {
		if entered == 0 {
			depth.Delete(id)
			return
		}
		depth.Store(id, entered)
	}
}

// Inside reports whether this goroutine is currently inside a transform.
func Inside() bool {
	current, ok := depth.Load(goroutineID())
	if !ok {
		return false
	}
	entered, _ := current.(int)
	return entered > 0
}

// Guard reports one guarded operation. It costs a map lookup only after a test
// installs a reporter.
func Guard(operation string) {
	if Impure != nil && Inside() {
		Impure(operation)
	}
}

func goroutineID() uint64 {
	var buffer [64]byte
	n := runtime.Stack(buffer[:], false)
	text := string(buffer[:n]) // "goroutine 123 [running]:"
	const prefix = "goroutine "
	if len(text) <= len(prefix) {
		return 0
	}
	text = text[len(prefix):]
	end := 0
	for end < len(text) && text[end] >= '0' && text[end] <= '9' {
		end++
	}
	id, err := strconv.ParseUint(text[:end], 10, 64)
	if err != nil {
		return 0
	}
	return id
}
