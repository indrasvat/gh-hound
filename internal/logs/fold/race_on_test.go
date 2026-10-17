//go:build race

package fold

// raceSlowdown scales wall-clock budgets in tests under the race detector.
const raceSlowdown = 5
