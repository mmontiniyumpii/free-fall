package engine

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mmontiniyumpii/free-fall/bot/internal/game"
)

// draw arma un game.Board desde un dibujo: la PRIMERA línea es la fila de arriba (6), la última la de abajo (0).
func draw(t *testing.T, lines ...string) game.Board {
	t.Helper()
	if len(lines) != game.Rows {
		t.Fatalf("draw: %d líneas", len(lines))
	}
	var b game.Board
	for i, line := range lines {
		line = strings.ReplaceAll(line, " ", "")
		for c, ch := range line {
			if ch != '.' {
				b[game.Rows-1-i][c] = game.Cell(ch - '0')
			}
		}
	}
	return b
}

const e = "........"

// drawBoard es un tablero lleno sin ningún cuatro en línea (el mismo de internal/game).
func drawBoard(t *testing.T) game.Board {
	return draw(t, "21122122", "11212211", "21221112", "12111222", "12221121", "12112111", "21222122")
}

func choose(b game.Board, me game.Player, opts Options) Result {
	return ChooseMove(b, me, game.LegalMoves(b), opts)
}

func TestWindows(t *testing.T) {
	if len(windows) != 107 {
		t.Fatalf("hay %d ventanas, el plan dice 107", len(windows))
	}
	count := map[string]int{}
	for _, w := range windows {
		// Cada ventana son 4 celdas distintas, consecutivas en una de las 4 direcciones.
		c0, r0 := int(w[0])/game.Rows, int(w[0])%game.Rows
		c1, r1 := int(w[1])/game.Rows, int(w[1])%game.Rows
		dc, dr := c1-c0, r1-r0
		for i := 1; i < 4; i++ {
			c, r := int(w[i])/game.Rows, int(w[i])%game.Rows
			if c != c0+i*dc || r != r0+i*dr {
				t.Fatalf("ventana %v no es una línea recta", w)
			}
		}
		count[fmt.Sprint(dc, dr)]++
	}
	want := map[string]int{"1 0": 35, "0 1": 32, "1 1": 20, "1 -1": 20}
	if fmt.Sprint(count) != fmt.Sprint(want) {
		t.Errorf("ventanas por dirección = %v, want %v (35 horizontales, 32 verticales, 40 diagonales)", count, want)
	}
}

func TestPositionMatchesBoardAndDropUndo(t *testing.T) {
	b := draw(t, e, e, e, e, "...2....", "...1..2.", "1..12.21")
	p := newPosition(b)
	for r := range game.Rows {
		for c := range game.Cols {
			if p.cells[at(c, r)] != uint8(b[r][c]) {
				t.Fatalf("cells[%d,%d] = %d, board = %d", c, r, p.cells[at(c, r)], b[r][c])
			}
		}
	}
	if p.heights != [game.Cols]uint8{1, 0, 0, 3, 1, 0, 2, 1} || p.moves != 8 {
		t.Errorf("heights = %v moves = %d", p.heights, p.moves)
	}

	// drop + undo deja la posición exactamente igual, en secuencias aleatorias.
	rng := rand.New(rand.NewPCG(1, 1))
	for range 200 {
		before := p
		var played []game.Column
		who := uint8(1)
		for range rng.IntN(20) {
			c := game.Column(rng.IntN(game.Cols))
			if !p.canPlay(c) {
				continue
			}
			p.drop(c, who)
			played = append(played, c)
			who = 3 - who
		}
		for i := len(played) - 1; i >= 0; i-- {
			p.undo(played[i])
		}
		if p != before {
			t.Fatal("drop/undo no restauró la posición")
		}
	}
}

func TestWinsAgreesWithGameRules(t *testing.T) {
	rng := rand.New(rand.NewPCG(2, 2))
	for range 500 {
		var b game.Board
		p := newPosition(b)
		who := game.Player(1)
		for len(game.LegalMoves(b)) > 0 {
			legal := game.LegalMoves(b)
			c := legal[rng.IntN(len(legal))]
			var row int
			b, row, _ = game.Apply(b, c, who)
			p.drop(c, uint8(who))
			if got, want := p.wins(c), game.FindWin(b, row, c) != nil; got != want {
				t.Fatalf("wins = %v, game.FindWin = %v en\n%v", got, want, b)
			}
			if game.FindWin(b, row, c) != nil {
				break
			}
			who = 3 - who
		}
	}
}

func TestEvaluate(t *testing.T) {
	if got := evaluate(&position{}, 1); got != 0 {
		t.Errorf("tablero vacío = %d, want 0", got)
	}
	// Tres propias con la cuarta libre suma; tres del rival resta más.
	mine := newPosition(draw(t, e, e, e, e, e, e, "111....."))
	if got := evaluate(&mine, 1); got <= 0 {
		t.Errorf("tres propias abiertas = %d, want > 0", got)
	}
	if got := evaluate(&mine, 2); got >= 0 {
		t.Errorf("tres del rival abiertas = %d, want < 0", got)
	}
	// Bonus del centro: una ficha en la columna 3 vale más que en la 0.
	center, edge := newPosition(draw(t, e, e, e, e, e, e, "...1....")), newPosition(draw(t, e, e, e, e, e, e, "1......."))
	if evaluate(&center, 1) <= evaluate(&edge, 1) {
		t.Errorf("centro = %d, borde = %d: el centro debe valer más", evaluate(&center, 1), evaluate(&edge, 1))
	}
	// Simetría: el tablero espejado vale lo mismo.
	b := draw(t, e, e, e, e, "..2.....", ".12.1...", "1121.2..")
	var mirror game.Board
	for r := range game.Rows {
		for c := range game.Cols {
			mirror[r][game.Cols-1-c] = b[r][c]
		}
	}
	pb, pm := newPosition(b), newPosition(mirror)
	if evaluate(&pb, 1) != evaluate(&pm, 1) {
		t.Errorf("evaluate no es simétrico: %d vs %d", evaluate(&pb, 1), evaluate(&pm, 1))
	}
}

func TestTactics(t *testing.T) {
	quick := Options{Budget: 300 * time.Millisecond}
	tests := []struct {
		name  string
		board game.Board
		me    game.Player
		want  []game.Column // cualquiera de estas
		avoid []game.Column // ninguna de estas
	}{
		{
			name:  "wins horizontally now",
			board: draw(t, e, e, e, e, e, "222.....", "111....."),
			me:    1, want: []game.Column{3},
		},
		{
			name:  "wins vertically now instead of blocking",
			board: draw(t, e, e, e, e, "1.......", "1.......", "1.222..."),
			me:    1, want: []game.Column{0},
		},
		{
			name:  "blocks the opponent",
			board: draw(t, e, e, e, e, e, "1.......", "1222...1"),
			me:    1, want: []game.Column{4},
		},
		{
			name:  "creates a double threat",
			board: draw(t, e, e, e, e, e, ".......2", "..11...2"),
			me:    1, want: []game.Column{1, 4},
		},
		{
			// El 2 gana en (1,3) en cuanto alguien llene (0,3): jugar la columna 3 le regala la partida.
			name:  "does not play under the opponent's winning cell",
			board: draw(t, e, e, e, e, e, "222.....", "121...11"),
			me:    1, avoid: []game.Column{3},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := choose(tc.board, tc.me, quick)
			if len(tc.want) > 0 && !slices.Contains(tc.want, r.Column) {
				t.Errorf("columna %d, want una de %v (%+v)", r.Column, tc.want, r)
			}
			if slices.Contains(tc.avoid, r.Column) {
				t.Errorf("columna %d, que pierde (%+v)", r.Column, r)
			}
		})
	}
}

// Con MaxDepth = 1 la búsqueda no ve la respuesta del rival: si bloquea, es por la red de seguridad.
func TestSafetyNetBlocksWithoutSearch(t *testing.T) {
	b := draw(t, e, e, e, e, e, "1.......", "1222...1")
	if r := choose(b, 1, Options{Budget: time.Second, MaxDepth: 1}); r.Column != 4 {
		t.Errorf("columna %d, want 4 (bloquear)", r.Column)
	}
}

// La raíz reconoce una victoria inmediata aunque se la llame sin pasar por la red de seguridad.
func TestRootScoresImmediateWin(t *testing.T) {
	s := searcher{pos: newPosition(draw(t, e, e, e, e, e, "222.....", "111.....")), hardStop: time.Now().Add(time.Hour)}
	order, n := rootOrder(&s.pos, []game.Column{0, 1, 2, 3, 4, 5, 6, 7}, 0, false)
	col, score, ok := s.searchRoot(2, 1, order, n)
	if !ok || col != 3 || score != winScore-1 {
		t.Errorf("searchRoot = columna %d score %d ok %v, want 3, %d, true", col, score, ok, winScore-1)
	}
}

// Iterative deepening devuelve la jugada de la última profundidad COMPLETA: el resultado con presupuesto
// tiene que ser el mismo que una búsqueda completa a la profundidad informada.
func TestBudgetedResultComesFromACompletedDepth(t *testing.T) {
	boards := []game.Board{
		draw(t, e, e, e, e, "...2....", "..121...", ".2112..."),
		draw(t, e, e, e, e, e, "..12....", ".2112..."),
		draw(t, e, e, e, "...1....", "...2.2..", "..112...", ".21121.."),
	}
	for i, b := range boards {
		r := choose(b, 1, Options{Budget: 60 * time.Millisecond})
		full := choose(b, 1, Options{Budget: time.Minute, MaxDepth: r.Depth})
		if r.Column != full.Column || r.Score != full.Score {
			t.Errorf("tablero %d: con presupuesto %+v, completa a profundidad %d %+v", i, r, r.Depth, full)
		}
	}
}

func TestOnlyOneLegalColumn(t *testing.T) {
	only6 := draw(t,
		"121212.1",
		"212121.2",
		"121212.1",
		"212121.2",
		"121212.1",
		"212121.2",
		"121212.1",
	)
	r := choose(only6, 2, Options{Budget: time.Second})
	if r.Column != 6 {
		t.Errorf("columna %d, la única legal es 6", r.Column)
	}
	if r.Elapsed > 50*time.Millisecond {
		t.Errorf("con una sola columna tardó %v: no debería buscar", r.Elapsed)
	}
}

func TestBoardWith55Cells(t *testing.T) {
	b := drawBoard(t)
	b[6][7] = game.Empty // queda una sola casilla, para el 2
	if r := choose(b, 2, Options{Budget: time.Second}); r.Column != 7 {
		t.Errorf("columna %d, want 7", r.Column)
	}
}

// Nunca devuelve una columna fuera de legal_moves, en 1 000 posiciones aleatorias.
func TestNeverIllegal(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 3))
	for i := range 1000 {
		var b game.Board
		who := game.Player(1)
		for range rng.IntN(game.Cells) {
			legal := game.LegalMoves(b)
			c := legal[rng.IntN(len(legal))]
			next, row, _ := game.Apply(b, c, who)
			if game.FindWin(next, row, c) != nil || len(game.LegalMoves(next)) == 0 {
				break // posición terminada: se queda con la anterior
			}
			b, who = next, 3-who
		}
		legal := game.LegalMoves(b)
		r := ChooseMove(b, who, legal, Options{Budget: 20 * time.Millisecond, MaxDepth: 1 + i%5})
		if !slices.Contains(legal, r.Column) {
			t.Fatalf("posición %d: columna %d, legal = %v\n%v", i, r.Column, legal, b)
		}
	}
}

// Guard: si legal_moves (del árbitro) no coincide con el tablero, la respuesta igual está en legal_moves.
func TestGuardUsesGivenLegalMoves(t *testing.T) {
	b := draw(t, "1.......", "2.......", "1.......", "2.......", "1.......", "2.......", "1.......")
	if r := ChooseMove(b, 2, []game.Column{0}, Options{Budget: 50 * time.Millisecond}); r.Column != 0 {
		t.Errorf("columna %d, want 0 (la única en legal, aunque el tablero diga que está llena)", r.Column)
	}
	if r := ChooseMove(b, 2, []game.Column{5}, Options{Budget: 50 * time.Millisecond}); r.Column != 5 {
		t.Errorf("columna %d, want 5", r.Column)
	}
	// Con varias: busca solo entre las de legal que el tablero permite (la 0 está llena).
	if r := ChooseMove(b, 2, []game.Column{0, 5}, Options{Budget: 50 * time.Millisecond}); r.Column != 5 {
		t.Errorf("columna %d, want 5", r.Column)
	}
}

func TestRespectsBudget(t *testing.T) {
	// Medio juego: la búsqueda no termina sola en el presupuesto.
	b := draw(t, e, e, e, e, "...2....", "..121...", ".2112...")
	for _, budget := range []time.Duration{100 * time.Millisecond, 1500 * time.Millisecond} {
		t.Run(budget.String(), func(t *testing.T) {
			start := time.Now()
			r := choose(b, 1, Options{Budget: budget})
			wall := time.Since(start)
			// Parada dura a budget + 250 ms (1 500 → 1 750 ms): con parseo y red, la jugada llega al árbitro
			// en menos de 1 800 ms (docs/TECNICO.md, D4). Tolerancia por chequear el reloj cada 2 048 nodos:
			// ~1 ms a velocidad real, ~50 ms con -race (ver race_on_test.go).
			tolerance := 25 * time.Millisecond
			if raceEnabled {
				tolerance = 150 * time.Millisecond
			}
			limit := budget + hardStopMargin + tolerance
			if wall > limit {
				t.Errorf("tardó %v, límite %v (%+v)", wall, limit, r)
			}
			if r.Depth < 1 {
				t.Errorf("no completó ninguna profundidad: %+v", r)
			}
		})
	}
}

func TestRespectsMaxDepth(t *testing.T) {
	for _, maxDepth := range []int{1, 2, 4, 6} {
		r := choose(game.Board{}, 1, Options{Budget: 10 * time.Second, MaxDepth: maxDepth})
		if r.Depth != maxDepth {
			t.Errorf("MaxDepth %d: Depth = %d", maxDepth, r.Depth)
		}
	}
	// Sin límite de profundidad, con tiempo, busca más hondo que easy.
	if r := choose(game.Board{}, 1, Options{Budget: 300 * time.Millisecond}); r.Depth <= 2 {
		t.Errorf("sin MaxDepth llegó solo a profundidad %d", r.Depth)
	}
}

func TestDeterministicWithMaxDepth(t *testing.T) {
	b := draw(t, e, e, e, e, e, "..12....", ".2112...")
	first := choose(b, 1, Options{Budget: 10 * time.Second, MaxDepth: 5})
	for range 3 {
		if r := choose(b, 1, Options{Budget: 10 * time.Second, MaxDepth: 5}); r.Column != first.Column || r.Score != first.Score {
			t.Fatalf("resultado distinto: %+v vs %+v", r, first)
		}
	}
}

// El motor es reentrante: el árbitro y la app lo llaman en paralelo. Con -race detecta estado compartido.
func TestConcurrentCallsAreIndependent(t *testing.T) {
	boards := []game.Board{
		{},
		draw(t, e, e, e, e, e, "..12....", ".2112..."),
		draw(t, e, e, e, e, e, "222.....", "111....2"),
		draw(t, e, e, e, e, e, ".......2", "..11...2"),
	}
	want := make([]Result, len(boards))
	for i, b := range boards {
		want[i] = choose(b, 1, Options{Budget: 10 * time.Second, MaxDepth: 5})
	}
	for i, b := range boards {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel() // los subtests corren al mismo tiempo, cada uno en su goroutine
			for range 5 {
				if r := choose(b, 1, Options{Budget: 10 * time.Second, MaxDepth: 5}); r.Column != want[i].Column || r.Score != want[i].Score {
					t.Errorf("en paralelo: %+v, solo: %+v", r, want[i])
				}
			}
		})
	}
}

// La búsqueda no pide memoria por nodo: AGENTS.md exige 0 allocs en el nodo.
func TestSearchDoesNotAllocate(t *testing.T) {
	s := searcher{pos: newPosition(draw(t, e, e, e, e, e, "..12....", ".2112...")), hardStop: time.Now().Add(time.Hour)}
	allocs := testing.AllocsPerRun(3, func() {
		s.negamax(6, 0, -infinity, infinity, 1)
	})
	if allocs != 0 {
		t.Errorf("negamax pidió %v asignaciones por búsqueda, want 0", allocs)
	}
}

func BenchmarkSearchDepth8(b *testing.B) {
	s := searcher{pos: newPosition(game.Board{}), hardStop: time.Now().Add(time.Hour)}
	b.ReportAllocs()
	for b.Loop() {
		s.nodes = 0
		s.negamax(8, 0, -infinity, infinity, 1)
	}
	b.ReportMetric(float64(s.nodes), "nodes/op")
}
