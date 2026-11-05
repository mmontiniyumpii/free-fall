package game

// Pruebas agregadas en la revisión (docs/revision-bot.md). Sin commitear hasta que el equipo las apruebe.

import (
	"reflect"
	"testing"
)

// Recorre las 107 ventanas posibles: con 4 fichas, FindWin las detecta desde cada una de las 4 casillas
// (cubre todos los bordes, esquinas, la fila 6, la columna 7 y las diagonales que cruzan el centro);
// con 3 fichas y la cuarta vacía, no hay victoria.
func TestAuditFindWinEveryWindowFromEveryCell(t *testing.T) {
	windows := 0
	for _, d := range directions {
		for r := range Rows {
			for c := range Cols {
				endR, endC := r+3*d.Row, c+3*d.Col
				if !inside(endR, endC) {
					continue
				}
				windows++
				var cells []Pos
				for i := range 4 {
					cells = append(cells, Pos{Row: r + i*d.Row, Col: c + i*d.Col})
				}
				for _, p := range []Player{1, 2} {
					var b Board
					for _, cell := range cells {
						b[cell.Row][cell.Col] = Cell(p)
					}
					for _, played := range cells {
						got := FindWin(b, played.Row, Column(played.Col))
						if !sameCells(got, cells) {
							t.Errorf("ventana %v dir %v, jugada en %v: FindWin = %v", cells, d, played, got)
						}
					}
					for missing := range 4 {
						three := b
						three[cells[missing].Row][cells[missing].Col] = Empty
						for i, played := range cells {
							if i == missing {
								continue
							}
							if got := FindWin(three, played.Row, Column(played.Col)); got != nil {
								t.Errorf("ventana %v con la casilla %v vacía: FindWin = %v, want nil", cells, cells[missing], got)
							}
						}
					}
				}
			}
		}
	}
	if windows != 107 {
		t.Errorf("hay %d ventanas, want 107 (35 + 32 + 40)", windows)
	}
}

func sameCells(got, want []Pos) bool {
	if len(got) != len(want) {
		return false
	}
	set := map[Pos]bool{}
	for _, p := range got {
		set[p] = true
	}
	for _, p := range want {
		if !set[p] {
			return false
		}
	}
	return true
}

func TestAuditFindWinHandDrawn(t *testing.T) {
	tests := []struct {
		name  string
		board Board
		row   int
		col   Column
		want  []Pos
	}{
		{
			// Diagonal \ que toca la fila 6 y la columna 7: (6,4) (5,5) (4,6) (3,7).
			name: "anti-diagonal touching row 6 and column 7",
			board: draw(t,
				"....1...",
				".....1..",
				"......1.",
				".......1",
				"......22",
				".....222",
				"....2122",
			),
			row: 3, col: 7,
			want: []Pos{{3, 7}, {4, 6}, {5, 5}, {6, 4}},
		},
		{
			// Diagonal / que termina en la esquina de arriba a la derecha (6,7).
			name: "diagonal ending in the top-right corner",
			board: draw(t,
				".......2",
				"......21",
				".....212",
				"....2121",
				"....1212",
				"....2121",
				"....1212",
			),
			row: 6, col: 7,
			want: []Pos{{3, 4}, {4, 5}, {5, 6}, {6, 7}},
		},
		{
			// La diagonal más larga cruza el centro: (0,1) ... (6,7). Jugada en el centro (3,4).
			name: "long diagonal through the center, detected from the middle",
			board: draw(t,
				"........",
				"........",
				"........",
				"....1...",
				"...12...",
				"..112...",
				"21221...",
			),
			row: 1, col: 2,
			want: []Pos{{0, 1}, {1, 2}, {2, 3}, {3, 4}},
		},
		{
			name: "horizontal on the top row ending in column 7",
			board: draw(t,
				"....1111",
				"....2222",
				"....1111",
				"....2222",
				"....1111",
				"....2222",
				"....1112",
			),
			row: 6, col: 7,
			want: []Pos{{6, 4}, {6, 5}, {6, 6}, {6, 7}},
		},
		{
			name: "vertical in column 7 reaching row 6",
			board: draw(t,
				".......2",
				".......2",
				".......2",
				".......2",
				".......1",
				".......1",
				"......11",
			),
			row: 6, col: 7,
			want: []Pos{{3, 7}, {4, 7}, {5, 7}, {6, 7}},
		},
		{
			name: "anti-diagonal ending in the bottom-right corner",
			board: draw(t,
				"........",
				"........",
				"........",
				"....2...",
				"....12..",
				"....112.",
				"....1112",
			),
			row: 0, col: 7,
			want: []Pos{{0, 7}, {1, 6}, {2, 5}, {3, 4}},
		},
		{
			name:  "seven in a row, played piece at the right end",
			board: draw(t, empty, empty, empty, empty, empty, "2222222.", "11111111"),
			row:   0, col: 7,
			want: []Pos{{0, 4}, {0, 5}, {0, 6}, {0, 7}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := FindWin(tc.board, tc.row, tc.col); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("FindWin(%d, %d) = %v, want %v", tc.row, tc.col, got, tc.want)
			}
		})
	}
}

// Apply apila de abajo hacia arriba en la columna 7 (el borde derecho) y rechaza la octava ficha.
func TestAuditApplyFillsColumn7(t *testing.T) {
	var b Board
	for want := range Rows {
		next, row, err := Apply(b, 7, Player(want%2+1))
		if err != nil || row != want || next[want][7] != Cell(want%2+1) {
			t.Fatalf("ficha %d: fila %d, err %v", want+1, row, err)
		}
		b = next
	}
	if _, row, err := Apply(b, 7, 1); err != ErrColumnFull || row != -1 {
		t.Errorf("columna 7 llena: fila %d, err %v", row, err)
	}
	if got := LegalMoves(b); reflect.DeepEqual(got, []Column{0, 1, 2, 3, 4, 5, 6, 7}) {
		t.Errorf("LegalMoves incluye la columna llena: %v", got)
	}
}

// Empate exactamente con la casilla 56: la última ficha llena el tablero sin hacer cuatro.
func TestAuditDrawOnTheLastCell(t *testing.T) {
	full := drawBoard(t)
	last := full[6][7]
	almost := full
	almost[6][7] = Empty
	if IsDraw(almost) {
		t.Fatal("55 fichas no es empate")
	}
	b, row, err := Apply(almost, 7, Player(last))
	if err != nil || row != 6 {
		t.Fatalf("Apply = fila %d, err %v", row, err)
	}
	if FindWin(b, row, 7) != nil || !IsDraw(b) || len(LegalMoves(b)) != 0 {
		t.Errorf("la ficha 56 sin cuatro debe ser empate: win=%v draw=%v legal=%v", FindWin(b, row, 7), IsDraw(b), LegalMoves(b))
	}
}
