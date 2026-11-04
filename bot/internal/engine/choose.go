package engine

import (
	"slices"
	"time"

	"github.com/mmontiniyumpii/free-fall/bot/internal/game"
)

const (
	// defaultBudget es THINK_MS por defecto, por si alguien llama con Budget = 0.
	defaultBudget = 1500 * time.Millisecond
	// hardStopMargin: la búsqueda se aborta en Budget + 250 ms (1 750 ms con el valor por defecto).
	// Sumando parseo y red, la jugada llega al árbitro antes de 1 800 ms (docs/TECNICO.md, D4).
	hardStopMargin = 250 * time.Millisecond
)

// Options configura una búsqueda.
type Options struct {
	Budget   time.Duration // no empieza una profundidad nueva pasado Budget; aborta a Budget + 250 ms
	MaxDepth int           // 0 = sin límite (torneo y "hard"); la dificultad de la app lo limita (docs/TECNICO.md, D1)
}

// Result es la jugada elegida y los datos de la búsqueda (para el log y last_bot).
type Result struct {
	Column  game.Column
	Depth   int   // última profundidad completada
	Nodes   int64 // posiciones visitadas
	Score   int   // valor de la jugada para el bot; ±(winScore − ply) si hay victoria o derrota forzada
	Elapsed time.Duration
}

// ChooseMove elige la columna para me. La respuesta siempre pertenece a legal (guard final).
//
//  1. Una sola columna legal: se juega sin buscar.
//  2. Red de seguridad: si me gana ahora, gana; si el rival gana en su próxima jugada, bloquea.
//  3. Iterative deepening: busca a profundidad 1, 2, 3... mientras haya tiempo y no supere MaxDepth,
//     y devuelve la mejor jugada de la última profundidad completa.
func ChooseMove(b game.Board, me game.Player, legal []game.Column, opts Options) Result {
	start := time.Now()
	if len(legal) == 0 {
		return Result{Elapsed: time.Since(start)} // no debería pasar: el árbitro y la API nunca piden jugar sin columnas
	}
	budget := opts.Budget
	if budget <= 0 {
		budget = defaultBudget
	}
	s := searcher{pos: newPosition(b), hardStop: start.Add(budget + hardStopMargin)}
	who := uint8(me)
	done := func(c game.Column, depth, score int) Result {
		return Result{Column: guard(c, legal), Depth: depth, Nodes: s.nodes, Score: score, Elapsed: time.Since(start)}
	}

	if len(legal) == 1 {
		return done(legal[0], 0, 0)
	}
	order, n := rootOrder(&s.pos, legal, 0, false)
	if n == 0 {
		return done(legal[0], 0, 0) // legal no coincide con el tablero: el guard decide
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

	maxDepth := game.Cells - s.pos.moves // nunca más profundo que las casillas libres
	if opts.MaxDepth > 0 && opts.MaxDepth < maxDepth {
		maxDepth = opts.MaxDepth
	}
	softStop := start.Add(budget)
	best, bestScore, depth := order[0], 0, 0
	for d := 1; d <= maxDepth; d++ {
		if d > 1 && time.Now().After(softStop) {
			break
		}
		// La mejor jugada de la profundidad anterior se prueba primero: suele seguir siendo la mejor y poda más.
		order, n = rootOrder(&s.pos, legal, best, depth > 0)
		col, score, ok := s.searchRoot(d, who, order, n)
		if !ok {
			break // abortada por tiempo: se queda con la profundidad anterior
		}
		best, bestScore, depth = col, score, d
		if score >= winScore-game.Cells || score <= -(winScore-game.Cells) {
			break // victoria o derrota forzada: buscar más hondo no cambia el resultado
		}
	}
	return done(best, depth, bestScore)
}

// guard garantiza que la columna devuelta esté en legal; si no, la primera legal.
func guard(c game.Column, legal []game.Column) game.Column {
	if slices.Contains(legal, c) {
		return c
	}
	return legal[0]
}
