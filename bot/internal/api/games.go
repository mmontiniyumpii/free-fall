package api

import (
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/mmontiniyumpii/free-fall/bot/internal/game"
)

// BotMove es la jugada que devuelve el elector, con los datos de la búsqueda para last_bot.
type BotMove struct {
	Column game.Column
	Depth  int
	Nodes  int64
}

// Chooser elige la jugada del bot. maxDepth = 0 significa sin límite (solo tiempo).
// En producción es el motor (paso 6); en los tests, un elector determinista.
type Chooser func(b game.Board, me game.Player, legal []game.Column, maxDepth int) BotMove

// maxDepthFor traduce la dificultad del contrato a la profundidad máxima de búsqueda (docs/TECNICO.md, D1).
var maxDepthFor = map[string]int{"easy": 2, "medium": 6, "hard": 0}

// localGame es una partida humano vs bot. Su mutex serializa las jugadas de esa partida
// (dos clicks seguidos se procesan uno detrás del otro) sin bloquear a las demás partidas.
type localGame struct {
	mu           sync.Mutex
	id           string
	board        game.Board
	history      []game.Column
	human, bot   game.Player
	difficulty   string
	toMove       game.Player
	status       string // "playing" | "won" | "draw"
	winner       game.Player
	winningCells []game.Pos
	lastBot      *lastBot
}

// apply juega col para p y actualiza estado, ganador y turno. Requiere g.mu tomado.
func (g *localGame) apply(col game.Column, p game.Player) error {
	b, row, err := game.Apply(g.board, col, p)
	if err != nil {
		return err
	}
	g.board = b
	g.history = append(g.history, col)
	if win := game.FindWin(b, row, col); win != nil {
		g.status, g.winner, g.winningCells = "won", p, win
		return nil
	}
	if len(g.history) == game.Cells { // 56 fichas sin ganador
		g.status = "draw"
		return nil
	}
	g.toMove = 3 - p // 3-1 = 2, 3-2 = 1
	return nil
}

// botPlay hace jugar al bot. Requiere g.mu tomado.
// Si el elector devuelve una columna ilegal o entra en panic, juega la primera legal:
// la app nunca recibe un error por un bug del motor.
func (g *localGame) botPlay(choose Chooser) {
	legal := game.LegalMoves(g.board)
	start := time.Now()
	move := safeChoose(choose, g, legal)
	elapsed := time.Since(start)
	if !slices.Contains(legal, move.Column) {
		slog.Error("bot chose an illegal column, playing first legal", "game_id", g.id, "column", move.Column)
		move = BotMove{Column: legal[0]}
	}
	if err := g.apply(move.Column, g.bot); err != nil {
		// No debería pasar: la columna está en legal. Se loguea en vez de ocultarlo.
		slog.Error("bot move rejected by rules", "game_id", g.id, "column", move.Column, "err", err)
		return
	}
	g.lastBot = &lastBot{Column: int(move.Column), Depth: move.Depth, Nodes: move.Nodes, ElapsedMS: elapsed.Milliseconds()}
	slog.Info("local bot move", "game_id", g.id, "column", move.Column, "depth", move.Depth, "nodes", move.Nodes,
		"elapsed_ms", elapsed.Milliseconds(), "difficulty", g.difficulty)
}

func safeChoose(choose Chooser, g *localGame, legal []game.Column) (m BotMove) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("bot chooser panicked, playing first legal", "game_id", g.id, "panic", r)
			m = BotMove{Column: legal[0]}
		}
	}()
	return choose(g.board, g.bot, legal, maxDepthFor[g.difficulty])
}

// ---- GameState (contrato en docs/api-local.md) ----

type lastBot struct {
	Column    int   `json:"column"`
	Depth     int   `json:"depth"`
	Nodes     int64 `json:"nodes"`
	ElapsedMS int64 `json:"elapsed_ms"`
}

// gameState es la respuesta de la API. Las listas son []int y no []game.Column:
// encoding/json serializa cualquier slice de uint8 como un string base64.
type gameState struct {
	ID           string     `json:"id"`
	Board        game.Board `json:"board"`   // array fijo: se serializa como [[...], ...]
	ToMove       *int       `json:"to_move"` // punteros: null cuando no aplica
	LegalMoves   []int      `json:"legal_moves"`
	Status       string     `json:"status"`
	Winner       *int       `json:"winner"`
	WinningCells [][2]int   `json:"winning_cells"` // nil → null
	History      []int      `json:"history"`
	Human        int        `json:"human"`
	Bot          int        `json:"bot"`
	Difficulty   string     `json:"difficulty"`
	LastBot      *lastBot   `json:"last_bot"`
}

// state arma el GameState. Requiere g.mu tomado. Copia todo: la respuesta se serializa fuera del mutex.
func (g *localGame) state() gameState {
	st := gameState{
		ID:         g.id,
		Board:      g.board,
		LegalMoves: []int{},
		Status:     g.status,
		History:    make([]int, len(g.history)),
		Human:      int(g.human),
		Bot:        int(g.bot),
		Difficulty: g.difficulty,
	}
	for i, c := range g.history {
		st.History[i] = int(c)
	}
	if g.lastBot != nil {
		lb := *g.lastBot
		st.LastBot = &lb
	}
	if g.status == "playing" {
		toMove := int(g.toMove)
		st.ToMove = &toMove
		for _, c := range game.LegalMoves(g.board) {
			st.LegalMoves = append(st.LegalMoves, int(c))
		}
	}
	if g.status == "won" {
		winner := int(g.winner)
		st.Winner = &winner
		for _, p := range g.winningCells {
			st.WinningCells = append(st.WinningCells, [2]int{p.Row, p.Col})
		}
	}
	return st
}
