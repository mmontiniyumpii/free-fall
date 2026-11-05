//go:build !race

package arbiter

// Sin -race, como corre el bot de verdad. Ver race_on_test.go.
const raceEnabled = false
