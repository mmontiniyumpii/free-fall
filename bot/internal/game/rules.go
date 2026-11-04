package game

import "errors"

// Errores de Apply. Se comparan con errors.Is; internal/api los traduce a "illegal_move" o "validation".
var (
	ErrOutOfRange    = errors.New("column out of range")
	ErrColumnFull    = errors.New("column is full")
	ErrInvalidPlayer = errors.New("player must be 1 or 2")
)

// LegalMoves devuelve las columnas que no están llenas, en orden ascendente.
// Nunca devuelve nil: con el tablero lleno devuelve un slice vacío (en JSON, [] y no null).
func LegalMoves(b Board) []Column {
	moves := make([]Column, 0, Cols)
	for c := range Cols {
		// Las fichas caen, así que una columna está llena si y solo si su fila de arriba está ocupada.
		if b[Rows-1][c] == Empty {
			moves = append(moves, Column(c))
		}
	}
	return moves
}

// Apply deja caer una ficha de p en la columna c y devuelve el tablero nuevo y la fila donde cayó.
// El tablero recibido no se modifica (Board es un array y se pasa por copia).
// Si la jugada no es válida devuelve el mismo tablero, fila -1 y el error.
func Apply(b Board, c Column, p Player) (Board, int, error) {
	if p != 1 && p != 2 {
		return b, -1, ErrInvalidPlayer
	}
	if int(c) >= Cols {
		return b, -1, ErrOutOfRange
	}
	for row := range Rows {
		if b[row][c] == Empty {
			b[row][c] = Cell(p)
			return b, row, nil
		}
	}
	return b, -1, ErrColumnFull
}

// directions son los 4 sentidos de una línea: horizontal, vertical, diagonal / y diagonal \.
// [...] deja que el compilador cuente los elementos: con [4] y un elemento de menos, Go completaría
// con {0, 0}, una dirección que nunca avanza, y FindWin quedaría en un bucle infinito.
var directions = [...]Pos{{Row: 0, Col: 1}, {Row: 1, Col: 0}, {Row: 1, Col: 1}, {Row: 1, Col: -1}}

// FindWin revisa si la ficha en (row, c) forma un cuatro en línea con fichas del mismo jugador.
// Devuelve exactamente 4 posiciones que incluyen (row, c), ordenadas de abajo hacia arriba
// (y de izquierda a derecha en horizontal), o nil si no hay línea.
// Si la línea tiene más de 4 fichas, devuelve la primera ventana de 4 que contiene (row, c).
func FindWin(b Board, row int, c Column) []Pos {
	col := int(c)
	if !inside(row, col) || b[row][col] == Empty {
		return nil
	}
	who := b[row][col]

	for _, d := range directions {
		// Retrocede hasta el extremo de la línea, en sentido contrario a d.
		start := Pos{Row: row, Col: col}
		for owns(b, who, start.Row-d.Row, start.Col-d.Col) {
			start = Pos{Row: start.Row - d.Row, Col: start.Col - d.Col}
		}
		// Desde ese extremo avanza en sentido d y junta todas las fichas seguidas.
		var line []Pos
		for p := start; owns(b, who, p.Row, p.Col); p = (Pos{Row: p.Row + d.Row, Col: p.Col + d.Col}) {
			line = append(line, p)
		}
		if len(line) < 4 {
			continue
		}
		// k es el índice de la ficha jugada dentro de la línea; la ventana arranca a lo sumo 3 antes.
		k := max(abs(row-start.Row), abs(col-start.Col))
		from := max(0, k-3)
		return line[from : from+4]
	}
	return nil
}

// IsDraw indica si las 56 casillas están llenas y nadie hizo cuatro en línea.
func IsDraw(b Board) bool {
	if len(LegalMoves(b)) > 0 {
		return false
	}
	for r := range Rows {
		for c := range Cols {
			if FindWin(b, r, Column(c)) != nil {
				return false
			}
		}
	}
	return true
}

func inside(row, col int) bool {
	return row >= 0 && row < Rows && col >= 0 && col < Cols
}

// owns indica si (row, col) está dentro del tablero y tiene una ficha de who.
func owns(b Board, who Cell, row, col int) bool {
	return inside(row, col) && b[row][col] == who
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
