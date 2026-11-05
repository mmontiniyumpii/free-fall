//go:build race

package arbiter

// Solo se compila con go test -race (la línea //go:build lo decide). El detector vuelve el motor
// mucho más lento, así que el test de punta a punta necesita más margen. Ver engine/race_on_test.go.
const raceEnabled = true
