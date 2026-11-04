package engine

import "github.com/mmontiniyumpii/free-fall/bot/internal/game"

// Pesos de la heurística (PLAN_BACKEND.md §5). Bloquear amenazas del rival pesa un poco más que armar las propias.
const (
	threeOpen    = 50  // 3 propias + 1 vacía en una ventana
	twoOpen      = 10  // 2 propias + 2 vacías
	rivalThree   = -60 // 3 del rival + 1 vacía
	centerWeight = 3   // por ficha en las columnas 3 y 4, las que más líneas tocan
)

// windows son las 107 ventanas de 4 casillas donde puede haber un cuatro en línea:
// 35 horizontales (7 filas × 5), 32 verticales (8 columnas × 4) y 40 diagonales (20 por sentido).
// Se calculan una sola vez al cargar el paquete y nunca se modifican: leerlas desde varias goroutines es seguro.
var windows = buildWindows()

func buildWindows() [][4]uint8 {
	var ws [][4]uint8
	for _, d := range [4][2]int{{1, 0}, {0, 1}, {1, 1}, {1, -1}} { // {dCol, dFila}
		for c := range game.Cols {
			for r := range game.Rows {
				endC, endR := c+3*d[0], r+3*d[1]
				if endC < 0 || endC >= game.Cols || endR < 0 || endR >= game.Rows {
					continue
				}
				var w [4]uint8
				for i := range 4 {
					w[i] = uint8(at(c+i*d[0], r+i*d[1]))
				}
				ws = append(ws, w)
			}
		}
	}
	return ws
}

// evaluate puntúa una posición sin ganador desde el punto de vista de me: positivo = mejor para me.
func evaluate(p *position, me uint8) int {
	rival := 3 - me
	score := 0
	for _, w := range windows {
		mine, theirs := 0, 0
		for _, i := range w {
			switch p.cells[i] {
			case me:
				mine++
			case rival:
				theirs++
			}
		}
		switch {
		case mine == 3 && theirs == 0:
			score += threeOpen
		case mine == 2 && theirs == 0:
			score += twoOpen
		case theirs == 3 && mine == 0:
			score += rivalThree
		}
	}
	for _, c := range [2]int{3, 4} {
		for r := range game.Rows {
			switch p.cells[at(c, r)] {
			case me:
				score += centerWeight
			case rival:
				score -= centerWeight
			}
		}
	}
	return score
}
