//go:build !race

package engine

// Sin -race (como corre el bot de verdad). Ver race_on_test.go.
const raceEnabled = false
