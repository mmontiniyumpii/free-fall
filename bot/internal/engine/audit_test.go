//go:build audit

package engine

// Arnés de la revisión (docs/revision-bot.md). Solo compila con -tags audit, así que npm test no lo corre.
//
//	go test -tags audit -run AuditTactics -v ./internal/engine
//	go test -tags audit -run AuditDepth -v ./internal/engine
//	go test -tags audit -run AuditSelfPlay -v -timeout 60m ./internal/engine

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/mmontiniyumpii/free-fall/bot/internal/game"
)

const think = 1500 * time.Millisecond

func TestAuditTactics(t *testing.T) {
	full := drawBoard(t)
	oneFree := full
	oneFree[5][7], oneFree[6][7] = game.Empty, game.Empty // 54 fichas, solo la columna 7
	twoLeft := full
	twoLeft[6][5], twoLeft[6][6] = game.Empty, game.Empty // 54 fichas, columnas 5 y 6
	tests := []struct {
		name  string
		board game.Board
		me    game.Player
		want  []game.Column
		avoid []game.Column
	}{
		{"a gana en 1 (diagonal en la columna 7)", draw(t, e, e, e, e, "......12", ".....122", "1...1221"), 1, []game.Column{7}, nil},
		{"b bloquea en 1 (columna 0)", draw(t, e, e, e, e, e, ".11.....", ".2221..."), 1, []game.Column{0}, nil},
		{"c no juega debajo de la casilla ganadora del rival", draw(t, e, e, e, e, e, ".....222", "11...121"), 1, nil, []game.Column{4}},
		{"d arma doble amenaza propia (gana en 3)", draw(t, e, e, e, e, e, "...22...", "...11..."), 1, []game.Column{2, 5}, nil},
		{"e impide la doble amenaza rival", draw(t, e, e, e, e, e, "...1....", "..22...1"), 1, []game.Column{1, 4}, nil},
		{"f una sola columna libre", oneFree, 1, []game.Column{7}, nil},
		{"g a 2 fichas de llenarse", twoLeft, 1, []game.Column{5, 6}, nil},
		{"h apertura", game.Board{}, 1, []game.Column{3, 4}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := choose(tc.board, tc.me, Options{Budget: think, MaxDepth: 0})
			t.Logf("columna %d profundidad %d nodos %d score %d %v", r.Column, r.Depth, r.Nodes, r.Score, r.Elapsed.Round(time.Millisecond))
			if len(tc.want) > 0 && !slices.Contains(tc.want, r.Column) {
				t.Errorf("columna %d, want %v", r.Column, tc.want)
			}
			if slices.Contains(tc.avoid, r.Column) {
				t.Errorf("columna %d, que pierde", r.Column)
			}
		})
	}
}

func TestAuditDepth(t *testing.T) {
	// Posiciones reales: dos motores a profundidad 6 juegan desde [3 4 2] y se mide en distintos momentos.
	var b game.Board
	who := game.Player(1)
	for _, c := range []game.Column{3, 4, 2} {
		b, _, _ = game.Apply(b, c, who)
		who = 3 - who
	}
	plies := 3
	for _, at := range []int{3, 8, 12, 16, 20, 24, 28, 32, 38, 44} {
		for plies < at {
			// Las posiciones se generan con baselineChoose para que sean siempre las mismas, cambie o no el motor.
			r := baselineChoose(b, who, game.LegalMoves(b), Options{Budget: time.Second, MaxDepth: 6})
			next, row, _ := game.Apply(b, r.Column, who)
			if game.FindWin(next, row, r.Column) != nil {
				t.Logf("la partida terminó en la jugada %d", plies+1)
				return
			}
			b, who, plies = next, 3-who, plies+1
		}
		for _, eng := range []struct {
			name string
			f    func(game.Board, game.Player, []game.Column, Options) Result
		}{{"actual  ", ChooseMove}, {"baseline", baselineChoose}} {
			r := eng.f(b, who, game.LegalMoves(b), Options{Budget: think})
			t.Logf("%2d fichas %s: profundidad %2d nodos %9d (%4.1f M/s) score %6d columna %d %v", plies, eng.name, r.Depth, r.Nodes,
				float64(r.Nodes)/max(r.Elapsed.Seconds(), 1e-9)/1e6, r.Score, r.Column, r.Elapsed.Round(time.Millisecond))
		}
	}
}

func replay(t *testing.T, moves []game.Column) (game.Board, game.Player) {
	t.Helper()
	var b game.Board
	who := game.Player(1)
	for _, c := range moves {
		next, row, err := game.Apply(b, c, who)
		if err != nil || game.FindWin(next, row, c) != nil {
			t.Fatalf("apertura inválida en %v: %v", moves, err)
		}
		b, who = next, 3-who
	}
	return b, who
}

// ---- Partidas en memoria (mismas reglas que el mock, sin HTTP) ----

// mover elige una jugada. Devuelve también el Result del motor (Depth = -1 si no es el motor).
type mover func(b game.Board, me game.Player, rng *rand.Rand) (game.Column, Result)

func engineMover(opts Options) mover {
	return func(b game.Board, me game.Player, _ *rand.Rand) (game.Column, Result) {
		r := ChooseMove(b, me, game.LegalMoves(b), opts)
		return r.Column, r
	}
}

// houseMover replica cmd/mock/house.go: casa-basico gana y bloquea cuatros inmediatos; si no, al azar.
func houseMover(basic bool) mover {
	return func(b game.Board, me game.Player, rng *rand.Rand) (game.Column, Result) {
		legal := game.LegalMoves(b)
		if basic {
			for _, p := range []game.Player{me, 3 - me} {
				for _, c := range legal {
					nb, row, _ := game.Apply(b, c, p)
					if game.FindWin(nb, row, c) != nil {
						return c, Result{Depth: -1}
					}
				}
			}
		}
		return legal[rng.IntN(len(legal))], Result{Depth: -1}
	}
}

type outcome struct {
	winner   game.Player // 0 empate
	plies    int
	depths   []int // profundidades del jugador bajo prueba (sin jugadas forzadas)
	maxThink time.Duration
	moves    []game.Column
}

func playGame(p1, p2 mover, opening []game.Column, test game.Player, seed uint64) outcome {
	rng := rand.New(rand.NewPCG(seed, seed^0x9e37))
	var b game.Board
	var o outcome
	who := game.Player(1)
	for ply := 0; ; ply++ {
		legal := game.LegalMoves(b)
		if len(legal) == 0 {
			return o
		}
		var c game.Column
		if ply < len(opening) {
			c = opening[ply]
		} else {
			m := p1
			if who == 2 {
				m = p2
			}
			var r Result
			c, r = m(b, who, rng)
			if who == test && r.Depth > 1 {
				o.depths = append(o.depths, r.Depth)
				o.maxThink = max(o.maxThink, r.Elapsed)
			}
			if !slices.Contains(legal, c) {
				panic(fmt.Sprintf("jugada ilegal %d de %d", c, who))
			}
		}
		next, row, _ := game.Apply(b, c, who)
		o.plies++
		o.moves = append(o.moves, c)
		if game.FindWin(next, row, c) != nil {
			o.winner = who
			return o
		}
		b, who = next, 3-who
	}
}

type match struct {
	name     string
	test     mover // jugador bajo prueba
	rival    mover
	games    int // la mitad abriendo, la mitad respondiendo
	openings int // plies de apertura al azar (mismas para los dos colores)
}

// runMatch juega en paralelo (4 a la vez) y devuelve victorias/empates/derrotas del jugador bajo prueba.
func runMatch(t *testing.T, m match) (w, d, l int) {
	t.Helper()
	type job struct {
		i       int
		opening []game.Column
		test    game.Player
	}
	jobs := make(chan job)
	var mu sync.Mutex
	var depthSum, depthN int
	var maxThink time.Duration
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				p1, p2 := m.test, m.rival
				if j.test == 2 {
					p1, p2 = m.rival, m.test
				}
				o := playGame(p1, p2, j.opening, j.test, uint64(j.i)+1)
				mu.Lock()
				res := "E"
				switch o.winner {
				case j.test:
					w++
					res = "G"
				case 0:
					d++
				default:
					l++
					res = "P"
				}
				for _, dd := range o.depths {
					depthSum += dd
					depthN++
				}
				maxThink = max(maxThink, o.maxThink)
				t.Logf("%s #%02d prueba=jugador%d %s en %2d jugadas apertura=%v jugadas=%v", m.name, j.i, j.test, res, o.plies, j.opening, o.moves)
				mu.Unlock()
			}
		}()
	}
	rng := rand.New(rand.NewPCG(42, 42))
	for i := 0; i < m.games; i += 2 {
		var opening []game.Column
		if m.openings > 0 {
			opening = randomOpening(rng, m.openings)
		}
		jobs <- job{i, opening, 1}
		jobs <- job{i + 1, opening, 2}
	}
	close(jobs)
	wg.Wait()
	avg := 0.0
	if depthN > 0 {
		avg = float64(depthSum) / float64(depthN)
	}
	t.Logf("RESUMEN %s: %d G / %d E / %d P, profundidad media %.1f, pensamiento máximo %v", m.name, w, d, l, avg, maxThink.Round(time.Millisecond))
	return w, d, l
}

func randomOpening(rng *rand.Rand, n int) []game.Column {
	for {
		var b game.Board
		who := game.Player(1)
		var moves []game.Column
		ok := true
		for range n {
			c := game.Column(rng.IntN(game.Cols))
			next, row, err := game.Apply(b, c, who)
			if err != nil || game.FindWin(next, row, c) != nil {
				ok = false
				break
			}
			moves = append(moves, c)
			b, who = next, 3-who
		}
		if ok {
			return moves
		}
	}
}

func TestAuditSelfPlayHouse(t *testing.T) {
	hard := engineMover(Options{Budget: think})
	for _, m := range []match{
		{name: "hard vs casa-aleatorio", test: hard, rival: houseMover(false), games: 20},
		{name: "hard vs casa-basico", test: hard, rival: houseMover(true), games: 20},
	} {
		if _, _, l := runMatch(t, m); l > 0 {
			t.Errorf("%s: %d derrotas", m.name, l)
		}
	}
}

func TestAuditSelfPlayDepth4(t *testing.T) {
	m := match{name: "hard vs MaxDepth=4", test: engineMover(Options{Budget: think}),
		rival: engineMover(Options{Budget: think, MaxDepth: 4}), games: 20, openings: 2}
	if _, _, l := runMatch(t, m); l > 0 {
		t.Errorf("%s: %d derrotas", m.name, l)
	}
}

// Repite una partida perdida y muestra, jugada a jugada, qué veía el motor hard.
func TestAuditAnalyzeLoss(t *testing.T) {
	moves := []game.Column{2, 7, 4, 3, 3, 4, 3, 3, 4, 4, 3, 4, 4, 0, 3, 7, 0, 7, 7, 7, 3, 4, 0, 0, 2, 2, 2, 2, 2, 2, 7, 7, 5, 5, 0, 0, 0, 6, 6, 6, 6, 6, 6, 6, 5, 5}
	var b game.Board
	who := game.Player(1)
	for i, c := range moves {
		if i >= 2 {
			r := choose(b, who, Options{Budget: think})
			d4 := choose(b, who, Options{Budget: think, MaxDepth: 4})
			t.Logf("jugada %2d jugador%d jugó %d | hard: col %d prof %2d score %7d | d4: col %d score %5d", i+1, who, c, r.Column, r.Depth, r.Score, d4.Column, d4.Score)
		}
		b, _, _ = game.Apply(b, c, who)
		who = 3 - who
	}
}

// ---- Baseline: la búsqueda tal como estaba antes de la poda de jugadas perdedoras (D16) ----
//
// Copia literal de negamax, searchRoot y ChooseMove del commit 94aceeb, con otros nombres,
// para medir el motor nuevo contra el de antes en las mismas condiciones.

type baseSearcher struct {
	pos      position
	nodes    int64
	hardStop time.Time
	stopped  bool
}

func (s *baseSearcher) negamax(depth, ply, alpha, beta int, who uint8) int {
	s.nodes++
	if s.nodes%checkEvery == 0 && time.Now().After(s.hardStop) {
		s.stopped = true
	}
	if s.stopped {
		return 0
	}
	if s.pos.moves == game.Cells {
		return 0
	}
	if depth == 0 {
		return evaluate(&s.pos, who)
	}

	best := -infinity
	for _, c := range centerFirst {
		if !s.pos.canPlay(c) {
			continue
		}
		s.pos.drop(c, who)
		var score int
		if s.pos.wins(c) {
			score = winScore - (ply + 1)
		} else {
			score = -s.negamax(depth-1, ply+1, -beta, -alpha, 3-who)
		}
		s.pos.undo(c)
		if s.stopped {
			return 0
		}
		if score > best {
			best = score
		}
		if best > alpha {
			alpha = best
		}
		if alpha >= beta {
			break
		}
	}
	return best
}

func (s *baseSearcher) searchRoot(depth int, me uint8, order [game.Cols]game.Column, n int) (best game.Column, bestScore int, ok bool) {
	best, bestScore = order[0], -infinity
	alpha := -infinity
	for i := range n {
		c := order[i]
		s.pos.drop(c, me)
		var score int
		if s.pos.wins(c) {
			score = winScore - 1
		} else {
			score = -s.negamax(depth-1, 1, -infinity, -alpha, 3-me)
		}
		s.pos.undo(c)
		if s.stopped {
			return best, bestScore, false
		}
		if score > bestScore {
			best, bestScore = c, score
		}
		if score > alpha {
			alpha = score
		}
	}
	return best, bestScore, true
}

func baselineChoose(b game.Board, me game.Player, legal []game.Column, opts Options) Result {
	start := time.Now()
	if len(legal) == 0 {
		return Result{Elapsed: time.Since(start)}
	}
	budget := opts.Budget
	if budget <= 0 {
		budget = defaultBudget
	}
	s := baseSearcher{pos: newPosition(b), hardStop: start.Add(budget + hardStopMargin)}
	who := uint8(me)
	done := func(c game.Column, depth, score int) Result {
		return Result{Column: guard(c, legal), Depth: depth, Nodes: s.nodes, Score: score, Elapsed: time.Since(start)}
	}
	if len(legal) == 1 {
		return done(legal[0], 0, 0)
	}
	order, n := rootOrder(&s.pos, legal, 0, false)
	if n == 0 {
		return done(legal[0], 0, 0)
	}
	for i := range n {
		if s.pos.winsWith(order[i], who) {
			return done(order[i], 1, winScore-1)
		}
	}
	for i := range n {
		if s.pos.winsWith(order[i], 3-who) {
			return done(order[i], 1, 0)
		}
	}
	maxDepth := game.Cells - s.pos.moves
	if opts.MaxDepth > 0 && opts.MaxDepth < maxDepth {
		maxDepth = opts.MaxDepth
	}
	softStop := start.Add(budget)
	best, bestScore, depth := order[0], 0, 0
	for d := 1; d <= maxDepth; d++ {
		if d > 1 && time.Now().After(softStop) {
			break
		}
		order, n = rootOrder(&s.pos, legal, best, depth > 0)
		col, score, ok := s.searchRoot(d, who, order, n)
		if !ok {
			break
		}
		best, bestScore, depth = col, score, d
		if score >= winScore-game.Cells || score <= -(winScore-game.Cells) {
			break
		}
	}
	return done(best, depth, bestScore)
}

func baselineMover(opts Options) mover {
	return func(b game.Board, me game.Player, _ *rand.Rand) (game.Column, Result) {
		r := baselineChoose(b, me, game.LegalMoves(b), opts)
		return r.Column, r
	}
}

// Motor actual contra baseline: mismas aperturas de 2 jugadas, cada una con los dos colores.
// Criterio de D16: el motor nuevo no puede perder más partidas que baseline.
func TestAuditVsBaseline(t *testing.T) {
	m := match{name: "nuevo vs baseline", test: engineMover(Options{Budget: think}),
		rival: baselineMover(Options{Budget: think}), games: 20, openings: 2}
	if w, _, l := runMatch(t, m); l > w {
		t.Errorf("%s: el motor nuevo perdió %d y ganó %d", m.name, l, w)
	}
}
