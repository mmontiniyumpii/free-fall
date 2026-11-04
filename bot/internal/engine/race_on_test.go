//go:build race

package engine

// La línea "//go:build race" hace que este archivo solo se compile con go test -race.
// El detector de carreras vuelve la búsqueda ~50 veces más lenta, así que 2 048 nodos
// (el intervalo entre consultas al reloj) pasan de ~1 ms a ~50 ms.
const raceEnabled = true
