package game

// Tipos compartidos por todo el bot (PLAN_BACKEND.md §4). Cambiarlos requiere avisar al equipo.

// Cell es el contenido de una casilla: 0 vacía, 1 jugador 1, 2 jugador 2.
type Cell uint8

// Player es 1 (abrió la partida) o 2 (respondió).
type Player uint8

// Column es el índice de columna, 0 a 7.
type Column uint8

// Dimensiones del tablero.
const Rows, Cols, Cells = 7, 8, 56

// Empty es una casilla vacía.
const Empty Cell = 0

// Board es el tablero: Board[fila][columna], fila 0 = abajo (mismo formato que el árbitro).
// Es un array, no un slice: al pasarlo a una función se copia entero, así que nadie modifica el tablero de otro.
type Board [Rows][Cols]Cell

// Pos es la posición de una casilla. Se agregó para FindWin (ver docs/TECNICO.md, D2).
type Pos struct {
	Row int
	Col int
}
