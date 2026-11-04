package arbiter

import (
	"testing"
	"time"

	"github.com/mmontiniyumpii/free-fall/bot/internal/engine"
	"github.com/mmontiniyumpii/free-fall/bot/internal/game"
)

// El torneo juega siempre sin límite de profundidad: la dificultad es solo de la app (docs/TECNICO.md, D1).
func TestEngineChooserNeverLimitsDepth(t *testing.T) {
	var got []engine.Options
	fake := func(_ game.Board, _ game.Player, legal []game.Column, opts engine.Options) engine.Result {
		got = append(got, opts)
		return engine.Result{Column: legal[len(legal)-1], Depth: 9, Nodes: 1234}
	}
	choice := engineChooser(fake, 1500*time.Millisecond)(Turn{You: 2, LegalMoves: []game.Column{1, 4}})

	if len(got) != 1 || got[0] != (engine.Options{Budget: 1500 * time.Millisecond, MaxDepth: 0}) {
		t.Errorf("opciones = %+v, want Budget 1.5s y MaxDepth 0", got)
	}
	if choice != (Choice{Column: 4, Depth: 9, Nodes: 1234}) {
		t.Errorf("choice = %+v", choice)
	}
}

// Con el motor real, la jugada es legal y trae los datos de la búsqueda.
func TestEngineChooserWithRealEngine(t *testing.T) {
	legal := game.LegalMoves(game.Board{})
	c := EngineChooser(50 * time.Millisecond)(Turn{You: 1, LegalMoves: legal})
	if int(c.Column) >= game.Cols || c.Depth < 1 || c.Nodes < 1 {
		t.Errorf("choice = %+v", c)
	}
}
