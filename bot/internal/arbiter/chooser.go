package arbiter

import (
	"time"

	"github.com/mmontiniyumpii/free-fall/bot/internal/engine"
	"github.com/mmontiniyumpii/free-fall/bot/internal/game"
)

// engineFunc es la firma de engine.ChooseMove. Recibirla como parámetro permite probar el adaptador con un motor falso.
type engineFunc func(b game.Board, me game.Player, legal []game.Column, opts engine.Options) engine.Result

// EngineChooser adapta el motor al loop del árbitro, con think de presupuesto (THINK_MS).
// El torneo juega siempre sin límite de profundidad (MaxDepth = 0): la dificultad es solo de la app.
func EngineChooser(think time.Duration) ChooseFunc {
	return engineChooser(engine.ChooseMove, think)
}

func engineChooser(choose engineFunc, think time.Duration) ChooseFunc {
	return func(t Turn) Choice {
		r := choose(t.Board, t.You, t.LegalMoves, engine.Options{Budget: think, MaxDepth: 0})
		return Choice{Column: r.Column, Depth: r.Depth, Nodes: r.Nodes}
	}
}
