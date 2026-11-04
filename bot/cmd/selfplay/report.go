package main

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sort"
	"sync/atomic"
	"time"

	"github.com/mmontiniyumpii/free-fall/bot/internal/game"
)

// gameRecord es lo que interesa de GET /api/games/{id}.
type gameRecord struct {
	GameID string       `json:"game_id"`
	Status string       `json:"status"`
	Moves  []moveRecord `json:"moves"`
	Result *gameResult  `json:"result"`
}

type moveRecord struct {
	By     int   `json:"by"`
	Column int   `json:"column"`
	MS     int64 `json:"ms"` // ms desde served_at hasta que llegó la jugada, medido por el árbitro
}

type gameResult struct {
	Winner *int   `json:"winner"`
	Reason string `json:"reason"`
}

// summary acumula los resultados de la corrida.
type summary struct {
	limit        time.Duration // jugada más lenta permitida: 1 800 ms + latencia simulada (docs/TECNICO.md, D5)
	games        int
	reasons      map[string]int // connect4, draw, timeout, illegal_move, absent
	winsByPlayer [3]int         // índices 1 y 2: quién abrió ganó
	maxMS        int64
	slowMoves    int
}

func newSummary(limit time.Duration) *summary {
	return &summary{limit: limit, reasons: map[string]int{}}
}

func (s *summary) add(g gameRecord) {
	s.games++
	if g.Result != nil {
		s.reasons[g.Result.Reason]++
		if w := g.Result.Winner; w != nil && (*w == 1 || *w == 2) {
			s.winsByPlayer[*w]++
		}
	}
	for _, m := range g.Moves {
		s.maxMS = max(s.maxMS, m.MS)
		if time.Duration(m.MS)*time.Millisecond > s.limit {
			s.slowMoves++
		}
	}
}

// failures devuelve por qué la corrida no cumple el criterio del paso 7; vacío = cumple.
func (s *summary) failures(errorLogs, warnLogs int64) []string {
	var fails []string
	if s.games == 0 {
		fails = append(fails, "no se jugó ninguna partida")
	}
	reasons := make([]string, 0, len(s.reasons))
	for r := range s.reasons {
		reasons = append(reasons, r)
	}
	sort.Strings(reasons) // orden fijo en la salida
	for _, r := range reasons {
		if r != "connect4" && r != "draw" {
			fails = append(fails, fmt.Sprintf("%d partidas terminaron por %s", s.reasons[r], r))
		}
	}
	if s.slowMoves > 0 {
		fails = append(fails, fmt.Sprintf("%d jugadas superaron %v", s.slowMoves, s.limit))
	}
	if errorLogs > 0 {
		fails = append(fails, fmt.Sprintf("%d logs ERROR en los loops", errorLogs))
	}
	if warnLogs > 0 {
		fails = append(fails, fmt.Sprintf("%d logs WARN en los loops", warnLogs))
	}
	return fails
}

// countingHandler envuelve un slog.Handler y cuenta los registros ERROR y WARN.
// Incrustar slog.Handler (campo sin nombre) hereda sus métodos; solo se reescriben los que cambian.
type countingHandler struct {
	slog.Handler
	errors, warns *atomic.Int64
}

func (h countingHandler) Handle(ctx context.Context, r slog.Record) error {
	switch {
	case r.Level >= slog.LevelError:
		h.errors.Add(1)
	case r.Level >= slog.LevelWarn:
		h.warns.Add(1)
	}
	return h.Handler.Handle(ctx, r)
}

// WithAttrs y WithGroup tienen que devolver otro countingHandler: si no, logger.With(...) perdería el conteo.
func (h countingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return countingHandler{Handler: h.Handler.WithAttrs(attrs), errors: h.errors, warns: h.warns}
}

func (h countingHandler) WithGroup(name string) slog.Handler {
	return countingHandler{Handler: h.Handler.WithGroup(name), errors: h.errors, warns: h.warns}
}

// randomOpening elige hasta maxMoves columnas al azar que no terminan la partida,
// para que dos motores deterministas no repitan siempre la misma partida.
func randomOpening(seed uint64, maxMoves int) []int {
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	var b game.Board
	opening := []int{}
	for i := range rng.IntN(maxMoves + 1) {
		legal := game.LegalMoves(b)
		c := legal[rng.IntN(len(legal))]
		next, row, err := game.Apply(b, c, game.Player(i%2+1))
		if err != nil || game.FindWin(next, row, c) != nil {
			break
		}
		b = next
		opening = append(opening, int(c))
	}
	return opening
}
