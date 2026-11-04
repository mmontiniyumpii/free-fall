package main

import (
	"testing"
	"time"

	"github.com/mmontiniyumpii/free-fall/bot/internal/arbiter"
	"github.com/mmontiniyumpii/free-fall/bot/internal/engine"
	"github.com/mmontiniyumpii/free-fall/bot/internal/game"
)

// Los tests de GET /health y de la API local viven en internal/api.

// fakeEngine registra con qué opciones lo llamaron y devuelve una jugada fija.
type fakeEngine struct {
	opts []engine.Options
}

func (f *fakeEngine) choose(_ game.Board, _ game.Player, legal []game.Column, opts engine.Options) engine.Result {
	f.opts = append(f.opts, opts)
	return engine.Result{Column: legal[len(legal)-1], Depth: 9, Nodes: 1234}
}

// El torneo juega siempre sin límite de profundidad: la dificultad es solo de la app (docs/TECNICO.md, D1).
func TestArbiterAlwaysPlaysWithoutDepthLimit(t *testing.T) {
	var f fakeEngine
	choose := arbiterChooser(f.choose, 1500*time.Millisecond)
	got := choose(arbiter.Turn{You: 2, LegalMoves: []game.Column{1, 4}})

	if len(f.opts) != 1 || f.opts[0] != (engine.Options{Budget: 1500 * time.Millisecond, MaxDepth: 0}) {
		t.Errorf("opciones = %+v, want Budget 1.5s y MaxDepth 0", f.opts)
	}
	if got != (arbiter.Choice{Column: 4, Depth: 9, Nodes: 1234}) {
		t.Errorf("choice = %+v", got)
	}
}

func TestLocalChooserPassesDifficultyDepth(t *testing.T) {
	var f fakeEngine
	choose := localChooser(f.choose, 1500*time.Millisecond)
	for _, maxDepth := range []int{2, 6, 0} {
		choose(game.Board{}, 1, []game.Column{3}, maxDepth)
	}
	for i, want := range []int{2, 6, 0} {
		if f.opts[i] != (engine.Options{Budget: 1500 * time.Millisecond, MaxDepth: want}) {
			t.Errorf("llamada %d: opciones = %+v, want MaxDepth %d", i, f.opts[i], want)
		}
	}
}
