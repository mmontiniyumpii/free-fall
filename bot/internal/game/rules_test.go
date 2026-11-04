package game

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

// draw convierte un dibujo del tablero "como se ve en pantalla" en un Board.
// Recibe 7 líneas: la PRIMERA es la fila de arriba (fila 6) y la ÚLTIMA la de abajo (fila 0).
// '.' vacío, '1' jugador 1, '2' jugador 2. Los espacios se ignoran.
func draw(t *testing.T, lines ...string) Board {
	t.Helper()
	if len(lines) != Rows {
		t.Fatalf("draw: %d líneas, se esperaban %d", len(lines), Rows)
	}
	var b Board
	for i, line := range lines {
		line = strings.ReplaceAll(line, " ", "")
		if len(line) != Cols {
			t.Fatalf("draw: línea %d tiene %d columnas, se esperaban %d", i, len(line), Cols)
		}
		row := Rows - 1 - i // la primera línea dibujada es la fila de arriba
		for c, ch := range line {
			switch ch {
			case '.':
				b[row][c] = Empty
			case '1', '2':
				b[row][c] = Cell(ch - '0')
			default:
				t.Fatalf("draw: carácter inválido %q", ch)
			}
		}
	}
	return b
}

const empty = "........"

// El tablero dibujado tiene que quedar con la fila 0 abajo, igual que el árbitro.
func TestDrawnBoardOrientation(t *testing.T) {
	b := draw(t,
		"2.......", // fila 6 (arriba)
		empty,
		empty,
		empty,
		empty,
		empty,
		"1......2", // fila 0 (abajo)
	)
	if b[0][0] != 1 || b[0][7] != 2 || b[6][0] != 2 {
		t.Fatalf("orientación incorrecta: b[0][0]=%d b[0][7]=%d b[6][0]=%d", b[0][0], b[0][7], b[6][0])
	}

	// Una ficha en un tablero vacío cae a la fila 0, que es la última línea del dibujo.
	got, row, err := Apply(Board{}, 3, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := draw(t, empty, empty, empty, empty, empty, empty, "...1....")
	if row != 0 || got != want {
		t.Errorf("Apply en tablero vacío: row=%d\n got=%v\nwant=%v", row, got, want)
	}
}

func TestApply(t *testing.T) {
	stacked := draw(t, empty, empty, empty, empty, empty, "...2....", "...1....")
	fullCol := draw(t, "1.......", "2.......", "1.......", "2.......", "1.......", "2.......", "1.......")

	tests := []struct {
		name    string
		board   Board
		col     Column
		player  Player
		wantRow int
		wantErr error
	}{
		{"empty board lands on row 0", Board{}, 0, 1, 0, nil},
		{"stacks on top of existing pieces", stacked, 3, 1, 2, nil},
		{"player 2 can move", stacked, 4, 2, 0, nil},
		{"last free cell of a column", draw(t, empty, "2.......", "1.......", "2.......", "1.......", "2.......", "1......."), 0, 1, 6, nil},
		{"full column", fullCol, 0, 2, -1, ErrColumnFull},
		{"column out of range", Board{}, 8, 1, -1, ErrOutOfRange},
		{"player 0 is invalid", Board{}, 0, 0, -1, ErrInvalidPlayer},
		{"player 3 is invalid", Board{}, 0, 3, -1, ErrInvalidPlayer},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			before := tc.board
			got, row, err := Apply(tc.board, tc.col, tc.player)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if row != tc.wantRow {
				t.Errorf("row = %d, want %d", row, tc.wantRow)
			}
			if tc.board != before {
				t.Error("Apply modificó el tablero de entrada")
			}
			if err != nil {
				if got != tc.board {
					t.Error("con error, Apply debe devolver el tablero sin cambios")
				}
				return
			}
			if got[row][tc.col] != Cell(tc.player) {
				t.Errorf("celda [%d][%d] = %d, want %d", row, tc.col, got[row][tc.col], tc.player)
			}
			// Solo cambió una celda.
			diff := 0
			for r := range Rows {
				for c := range Cols {
					if got[r][c] != tc.board[r][c] {
						diff++
					}
				}
			}
			if diff != 1 {
				t.Errorf("cambiaron %d celdas, want 1", diff)
			}
		})
	}
}

func TestLegalMoves(t *testing.T) {
	tests := []struct {
		name  string
		board Board
		want  []Column
	}{
		{"empty board", Board{}, []Column{0, 1, 2, 3, 4, 5, 6, 7}},
		{
			"full columns are excluded",
			draw(t, "..1..2..", "..2..1..", "..1..2..", "..2..1..", "..1..2..", "..2..1..", "..1..2.."),
			[]Column{0, 1, 3, 4, 6, 7},
		},
		{"partially filled column is legal", draw(t, empty, empty, empty, empty, empty, "1.......", "2......."), []Column{0, 1, 2, 3, 4, 5, 6, 7}},
		{"full board has no moves", drawBoard(t), []Column{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := LegalMoves(tc.board)
			// nil y []Column{} se serializan distinto en JSON (null vs []): la API necesita [].
			if got == nil {
				t.Fatal("LegalMoves devolvió nil; debe ser un slice vacío")
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("LegalMoves = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFindWin(t *testing.T) {
	tests := []struct {
		name  string
		board Board
		row   int
		col   Column
		want  []Pos
	}{
		{
			name:  "horizontal on bottom row",
			board: draw(t, empty, empty, empty, empty, empty, empty, "..1111.."),
			row:   0, col: 4,
			want: []Pos{{0, 2}, {0, 3}, {0, 4}, {0, 5}},
		},
		{
			name:  "horizontal touching right edge",
			board: draw(t, empty, empty, empty, "....2222", "....1111", "....2222", "....1111"),
			row:   2, col: 7,
			want: []Pos{{2, 4}, {2, 5}, {2, 6}, {2, 7}},
		},
		{
			name:  "vertical",
			board: draw(t, empty, empty, empty, ".2......", ".2......", ".2......", "12......"),
			row:   3, col: 1,
			want: []Pos{{0, 1}, {1, 1}, {2, 1}, {3, 1}},
		},
		{
			name:  "vertical reaching the top row",
			board: draw(t, ".......1", ".......1", ".......1", ".......1", ".......2", ".......2", ".......2"),
			row:   6, col: 7,
			want: []Pos{{3, 7}, {4, 7}, {5, 7}, {6, 7}},
		},
		{
			name: "diagonal going up-right (/)",
			board: draw(t,
				empty,
				empty,
				empty,
				".....1..",
				"....12..",
				"...122..",
				"..1222..",
			),
			row: 3, col: 5,
			want: []Pos{{0, 2}, {1, 3}, {2, 4}, {3, 5}},
		},
		{
			name: "diagonal going up-left (\\) detected from the middle piece",
			board: draw(t,
				empty,
				empty,
				empty,
				"2.......",
				"12......",
				"112.....",
				"1112....",
			),
			row: 2, col: 1,
			want: []Pos{{0, 3}, {1, 2}, {2, 1}, {3, 0}}, // siempre de abajo hacia arriba
		},
		{
			name: "diagonal from the corner",
			board: draw(t,
				empty,
				empty,
				empty,
				"...1....",
				"..12....",
				".122....",
				"1222....",
			),
			row: 0, col: 0,
			want: []Pos{{0, 0}, {1, 1}, {2, 2}, {3, 3}},
		},
		{
			name:  "five in a row returns exactly 4 cells including the played one",
			board: draw(t, empty, empty, empty, empty, empty, "222.....", "11111..."),
			row:   0, col: 2,
			want: []Pos{{0, 0}, {0, 1}, {0, 2}, {0, 3}},
		},
		{
			name:  "three in a row is not a win",
			board: draw(t, empty, empty, empty, empty, empty, empty, "..111.2."),
			row:   0, col: 4,
			want: nil,
		},
		{
			name: "no wrap-around between rows",
			// Fila 0 columnas 6-7 y fila 1 columnas 0-1: 4 fichas "seguidas" en memoria, pero no en el tablero.
			board: draw(t, empty, empty, empty, empty, empty, "11......", "222..211"),
			row:   0, col: 7,
			want: nil,
		},
		{
			name:  "opponent pieces do not count",
			board: draw(t, empty, empty, empty, empty, empty, empty, "1112...."),
			row:   0, col: 3,
			want: nil,
		},
		{
			name:  "empty cell has no win",
			board: draw(t, empty, empty, empty, empty, empty, empty, "1111...."),
			row:   0, col: 5,
			want: nil,
		},
		{
			name:  "row out of range",
			board: Board{},
			row:   7, col: 0,
			want: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FindWin(tc.board, tc.row, tc.col)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("FindWin(row=%d, col=%d) = %v, want %v", tc.row, tc.col, got, tc.want)
			}
		})
	}
}

// drawBoard es un tablero lleno (28 fichas de cada jugador) sin ningún cuatro en línea.
func drawBoard(t *testing.T) Board {
	return draw(t,
		"21122122",
		"11212211",
		"21221112",
		"12111222",
		"12221121",
		"12112111",
		"21222122",
	)
}

func TestIsDraw(t *testing.T) {
	full := drawBoard(t)

	// Chequeo independiente de FindWin: recorre todas las ventanas de 4 a fuerza bruta.
	if hasAnyFour(full) {
		t.Fatal("drawBoard tiene un cuatro en línea: el fixture del test está mal")
	}

	almost := full
	almost[6][7] = Empty

	withWinner := full
	withWinner[0][0], withWinner[0][1], withWinner[0][2], withWinner[0][3] = 1, 1, 1, 1

	tests := []struct {
		name  string
		board Board
		want  bool
	}{
		{"full board without winner", full, true},
		{"empty board", Board{}, false},
		{"55 cells filled", almost, false},
		{"full board with a winner", withWinner, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsDraw(tc.board); got != tc.want {
				t.Errorf("IsDraw = %v, want %v", got, tc.want)
			}
		})
	}
}

func hasAnyFour(b Board) bool {
	for r := range Rows {
		for c := range Cols {
			p := b[r][c]
			if p == Empty {
				continue
			}
			for _, d := range [][2]int{{0, 1}, {1, 0}, {1, 1}, {1, -1}} {
				n := 1
				for n < 4 {
					rr, cc := r+d[0]*n, c+d[1]*n
					if rr < 0 || rr >= Rows || cc < 0 || cc >= Cols || b[rr][cc] != p {
						break
					}
					n++
				}
				if n == 4 {
					return true
				}
			}
		}
	}
	return false
}

// El fixture del contrato (docs/fixtures/game-state.example.json) debe ser coherente con las reglas:
// reconstruir el tablero jugando su history tiene que dar exactamente su board.
func TestFixtureMatchesRules(t *testing.T) {
	data, err := os.ReadFile("../../../docs/fixtures/game-state.example.json")
	if err != nil {
		t.Fatal(err)
	}
	var fx struct {
		Board      [Rows][Cols]Cell `json:"board"`
		History    []Column         `json:"history"`
		LegalMoves []Column         `json:"legal_moves"`
		ToMove     Player           `json:"to_move"`
	}
	if err := json.Unmarshal(data, &fx); err != nil {
		t.Fatal(err)
	}

	var b Board
	p := Player(1)
	for i, c := range fx.History {
		var row int
		b, row, err = Apply(b, c, p)
		if err != nil {
			t.Fatalf("jugada %d (columna %d): %v", i, c, err)
		}
		if FindWin(b, row, c) != nil {
			t.Fatalf("jugada %d (columna %d) gana, pero el fixture dice playing", i, c)
		}
		p = 3 - p // alterna 1 ↔ 2
	}
	if b != Board(fx.Board) {
		t.Errorf("board reconstruido desde history no coincide con el fixture\n got=%v\nwant=%v", b, fx.Board)
	}
	if p != fx.ToMove {
		t.Errorf("to_move = %d, want %d", fx.ToMove, p)
	}
	if got := LegalMoves(b); !reflect.DeepEqual(got, fx.LegalMoves) {
		t.Errorf("legal_moves = %v, fixture dice %v", got, fx.LegalMoves)
	}
}
