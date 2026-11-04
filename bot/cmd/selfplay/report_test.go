package main

import (
	"bytes"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mmontiniyumpii/free-fall/bot/internal/game"
)

func record(reason string, winner int, ms ...int64) gameRecord {
	r := gameRecord{Result: &gameResult{Reason: reason}}
	if winner != 0 {
		r.Result.Winner = &winner
	}
	for i, m := range ms {
		r.Moves = append(r.Moves, moveRecord{By: i%2 + 1, MS: m})
	}
	return r
}

func TestSummaryAddAndFailures(t *testing.T) {
	limit := 2100 * time.Millisecond // 1 800 + 300 de latencia simulada (docs/TECNICO.md, D5)

	tests := []struct {
		name    string
		games   []gameRecord
		errors  int64
		warns   int64
		wantOK  bool
		wantMsg string
	}{
		{"clean run", []gameRecord{record("connect4", 1, 2000, 1900), record("draw", 0, 1800)}, 0, 0, true, ""},
		{"no games", nil, 0, 0, false, "ninguna partida"},
		{"slow move", []gameRecord{record("connect4", 2, 2000, 2101)}, 0, 0, false, "1 jugadas superaron"},
		{"lost on time", []gameRecord{record("timeout", 1, 3200)}, 0, 0, false, "timeout"},
		{"illegal move", []gameRecord{record("illegal_move", 2, 100)}, 0, 0, false, "illegal_move"},
		{"absent", []gameRecord{record("absent", 2)}, 0, 0, false, "absent"},
		{"error logs", []gameRecord{record("connect4", 1, 100)}, 2, 0, false, "2 logs ERROR"},
		{"warn logs", []gameRecord{record("connect4", 1, 100)}, 0, 1, false, "1 logs WARN"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newSummary(limit)
			for _, g := range tc.games {
				s.add(g)
			}
			fails := s.failures(tc.errors, tc.warns)
			if ok := len(fails) == 0; ok != tc.wantOK {
				t.Fatalf("failures = %v, want ok = %v", fails, tc.wantOK)
			}
			if tc.wantMsg != "" && !strings.Contains(strings.Join(fails, "; "), tc.wantMsg) {
				t.Errorf("failures = %v, want que mencione %q", fails, tc.wantMsg)
			}
		})
	}
}

func TestSummaryTracksMaxAndWinners(t *testing.T) {
	s := newSummary(2100 * time.Millisecond)
	s.add(record("connect4", 1, 1200, 1999))
	s.add(record("connect4", 2, 2050, 300))
	s.add(record("draw", 0, 10))
	if s.games != 3 || s.maxMS != 2050 || s.reasons["connect4"] != 2 || s.reasons["draw"] != 1 {
		t.Errorf("summary = %+v", s)
	}
	if s.winsByPlayer[1] != 1 || s.winsByPlayer[2] != 1 {
		t.Errorf("victorias por jugador = %v", s.winsByPlayer)
	}
}

// countingHandler cuenta ERROR y WARN también en los loggers derivados con With (los que usan los loops).
func TestCountingHandler(t *testing.T) {
	var errs, warns atomic.Int64
	var buf bytes.Buffer
	logger := slog.New(countingHandler{Handler: slog.NewTextHandler(&buf, nil), errors: &errs, warns: &warns}).With("team", "A")
	logger.Info("turn")
	logger.Warn("move rejected")
	logger.Error("arbiter loop")
	logger.WithGroup("g").Error("otra")
	if errs.Load() != 2 || warns.Load() != 1 {
		t.Errorf("errors = %d, warns = %d; want 2 y 1", errs.Load(), warns.Load())
	}
	if !strings.Contains(buf.String(), "team=A") {
		t.Errorf("el handler no delega en el original:\n%s", buf.String())
	}
}

// La apertura se juega con las reglas reales: tiene que ser legal y no terminar la partida
// (el mock rechaza una apertura que gana). Con maxMoves = 7 una apertura aleatoria puede ganar seguido.
func TestRandomOpeningIsLegalAndNeverWins(t *testing.T) {
	lengths := map[int]bool{}
	for seed := range uint64(2000) {
		opening := randomOpening(seed, 7)
		lengths[len(opening)] = true
		var b game.Board
		for i, c := range opening {
			next, row, err := game.Apply(b, game.Column(c), game.Player(i%2+1))
			if err != nil {
				t.Fatalf("semilla %d: apertura %v ilegal en la jugada %d: %v", seed, opening, i, err)
			}
			if game.FindWin(next, row, game.Column(c)) != nil {
				t.Fatalf("semilla %d: la apertura %v gana en la jugada %d", seed, opening, i)
			}
			b = next
		}
	}
	if !lengths[0] || !lengths[7] {
		t.Errorf("largos de apertura vistos = %v; want desde 0 hasta 7", lengths)
	}
}
