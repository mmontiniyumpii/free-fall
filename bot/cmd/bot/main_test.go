package main

import (
	"testing"
	"time"

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

// El test de que el árbitro juega siempre con MaxDepth = 0 vive en internal/arbiter (chooser_test.go).

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
