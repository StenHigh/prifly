//go:build race

package runtime

// underRaceDetector is true in a `-race` build. Timing budgets that describe
// what a person waits for at a terminal do not survive it: every read is
// roughly an order of magnitude slower, and the engine's own cooperative
// deadline stops a query long before such a budget could mean anything.
const underRaceDetector = true
