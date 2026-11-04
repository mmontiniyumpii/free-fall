package main

import (
	"time"

	"github.com/mmontiniyumpii/free-fall/bot/internal/game"
)

// Límites del árbitro según las bases y el OpenAPI.
const (
	deadline    = 3000 * time.Millisecond // para responder, contados desde served_at
	absentAfter = 30 * time.Second        // sin recoger el turno → derrota por ausencia
)

// team es un equipo del mock: cada token distinto que llega es un equipo nuevo.
type team struct {
	id   string
	name string
	// notify avisa que el equipo tiene un turno nuevo. Tiene buffer de 1: un aviso puede quedar
	// guardado si nadie está esperando, y el próximo long poll lo encuentra al empezar.
	notify     chan struct{}
	submission *submissionRecord
}

type submissionRecord struct {
	RepoURL      string `json:"repo_url"`
	Commit       string `json:"commit"`
	RegisteredAt string `json:"registered_at"`
}

type moveRecord struct {
	Turn   int         `json:"turn"`
	By     game.Player `json:"by"`
	Column int         `json:"column"`
	MS     int64       `json:"ms"`
}

type gameResult struct {
	Winner       *game.Player `json:"winner"` // puntero para poder serializar null en empate
	Reason       string       `json:"reason"`
	WinningCells [][2]int     `json:"winning_cells,omitempty"`
}

// mockGame es una partida. Todos sus campos se leen y escriben con server.mu tomado.
type mockGame struct {
	id      string
	kind    string
	house   string     // rival de la casa
	players [3]*team   // índices 1 y 2; nil = la casa
	board   game.Board // board[fila][columna], fila 0 abajo
	moves   []moveRecord
	toMove  game.Player
	// availableAt es cuándo quedó disponible el turno actual; servedAt, cuándo se entregó por /api/next.
	availableAt time.Time
	servedAt    time.Time
	startedAt   time.Time
	finishedAt  time.Time
	result      *gameResult // nil mientras se juega
}

func (g *mockGame) playing() bool { return g.result == nil }

// pendingFor indica si la partida espera una jugada de t.
func (g *mockGame) pendingFor(t *team) bool {
	return g.playing() && g.players[g.toMove] == t
}

// sideOf devuelve qué número de jugador es t en la partida (0 si no juega en ella).
func (g *mockGame) sideOf(t *team) game.Player {
	for p := game.Player(1); p <= 2; p++ {
		if g.players[p] == t {
			return p
		}
	}
	return 0
}

// finish cierra la partida. winner 0 = empate.
func (g *mockGame) finish(now time.Time, winner game.Player, reason string, cells []game.Pos) {
	r := &gameResult{Reason: reason}
	if winner != 0 {
		r.Winner = &winner
	}
	for _, c := range cells {
		r.WinningCells = append(r.WinningCells, [2]int{c.Row, c.Col})
	}
	g.result = r
	g.finishedAt = now
}

// play aplica la jugada de side. Devuelve la línea ganadora si la hubo, o el error de game.Apply si es ilegal.
func (g *mockGame) play(now time.Time, side game.Player, col game.Column, ms int64) ([]game.Pos, error) {
	b, row, err := game.Apply(g.board, col, side)
	if err != nil {
		return nil, err
	}
	g.board = b
	g.moves = append(g.moves, moveRecord{Turn: len(g.moves) + 1, By: side, Column: int(col), MS: ms})

	if win := game.FindWin(b, row, col); win != nil {
		g.finish(now, side, "connect4", win)
		return win, nil
	}
	if len(g.moves) == game.Cells { // sin ganador y 56 fichas: empate
		g.finish(now, 0, "draw", nil)
		return nil, nil
	}
	g.toMove = 3 - side
	g.availableAt = now
	g.servedAt = time.Time{}
	return nil, nil
}

// expireAbsent aplica la derrota por ausencia si el turno lleva más de 30 s sin que el equipo lo recoja.
func (g *mockGame) expireAbsent(now time.Time) {
	if g.playing() && g.players[g.toMove] != nil && g.servedAt.IsZero() && now.Sub(g.availableAt) > absentAfter {
		g.finish(now, 3-g.toMove, "absent", nil)
	}
}
