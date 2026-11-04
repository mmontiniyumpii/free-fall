package engine

import "github.com/mmontiniyumpii/free-fall/bot/internal/game"

// position es la representación interna del motor: arrays fijos, sin slices ni punteros,
// para que la búsqueda juegue y deshaga jugadas sin pedir memoria.
//
// cells[col*7 + fila] (por columnas, fila 0 abajo): así la casilla libre de una columna es
// col*7 + heights[col], sin recorrer nada. Esta representación vive solo dentro de engine;
// hacia afuera siempre se usa game.Board.
type position struct {
	cells   [game.Cells]uint8 // 0 vacía, 1 o 2
	heights [game.Cols]uint8  // fichas en cada columna
	moves   int               // fichas en el tablero
}

// at es el índice en cells de la columna c, fila r.
func at(c, r int) int { return c*game.Rows + r }

// newPosition copia un game.Board a la representación interna. La altura de cada columna
// es la fila más alta ocupada + 1 (las fichas caen, así que no hay huecos debajo).
func newPosition(b game.Board) position {
	var p position
	for c := range game.Cols {
		for r := range game.Rows {
			if b[r][c] != game.Empty {
				p.cells[at(c, r)] = uint8(b[r][c])
				p.heights[c] = uint8(r + 1)
				p.moves++
			}
		}
	}
	return p
}

func (p *position) canPlay(c game.Column) bool { return p.heights[c] < game.Rows }

// drop juega una ficha de who en c. Recibe *position (puntero) para modificar la posición en el lugar.
func (p *position) drop(c game.Column, who uint8) {
	p.cells[at(int(c), int(p.heights[c]))] = who
	p.heights[c]++
	p.moves++
}

// undo deshace la última ficha de la columna c.
func (p *position) undo(c game.Column) {
	p.heights[c]--
	p.cells[at(int(c), int(p.heights[c]))] = 0
	p.moves--
}

// wins indica si la última ficha jugada en c completó cuatro en línea.
// Cuenta fichas iguales hacia los dos lados en cada una de las 4 direcciones.
func (p *position) wins(c game.Column) bool {
	col, row := int(c), int(p.heights[c])-1
	who := p.cells[at(col, row)]
	for _, d := range [4][2]int{{1, 0}, {0, 1}, {1, 1}, {1, -1}} { // {dCol, dFila}
		n := 1
		for _, sign := range [2]int{1, -1} {
			cc, rr := col+sign*d[0], row+sign*d[1]
			for cc >= 0 && cc < game.Cols && rr >= 0 && rr < game.Rows && p.cells[at(cc, rr)] == who {
				n++
				cc, rr = cc+sign*d[0], rr+sign*d[1]
			}
		}
		if n >= 4 {
			return true
		}
	}
	return false
}

// winsWith indica si jugar c con who gana. Deja la posición como estaba.
func (p *position) winsWith(c game.Column, who uint8) bool {
	p.drop(c, who)
	w := p.wins(c)
	p.undo(c)
	return w
}
