package arbiter

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/mmontiniyumpii/free-fall/bot/internal/game"
)

// ErrMalformed indica una respuesta del árbitro que no cumple el OpenAPI.
var ErrMalformed = errors.New("malformed arbiter response")

// Turn es un turno pendiente de GET /api/next, ya validado.
type Turn struct {
	GameID     string
	MatchID    *string // nil en prácticas (match_id: null)
	Kind       string
	You        game.Player
	Opponent   string
	Number     int // campo "turn": número de la jugada que sigue, desde 1
	Board      game.Board
	Moves      []game.Column
	LastMove   *LastMove // nil en el primer turno (last_move: null)
	LegalMoves []game.Column
	Deadline   time.Duration
	ServedAt   string // se guarda solo para logs: el bot nunca calcula tiempo a partir de served_at
}

// LastMove es la última jugada del rival.
type LastMove struct {
	By     game.Player
	Column game.Column
}

// rawNext refleja el JSON tal como llega. board es [][]int y no un array fijo:
// al decodificar en [7][8]int, encoding/json rellena con ceros las filas cortas sin avisar.
type rawNext struct {
	Status   string  `json:"status"`
	GameID   string  `json:"game_id"`
	MatchID  *string `json:"match_id"`
	Kind     string  `json:"kind"`
	You      int     `json:"you"`
	Opponent string  `json:"opponent"`
	Turn     int     `json:"turn"`
	Board    [][]int `json:"board"`
	Moves    []int   `json:"moves"`
	LastMove *struct {
		By     int `json:"by"`
		Column int `json:"column"`
	} `json:"last_move"`
	LegalMoves []int  `json:"legal_moves"`
	DeadlineMS int    `json:"deadline_ms"`
	ServedAt   string `json:"served_at"`
}

// ParseNext interpreta la respuesta de GET /api/next. Devuelve ok = true si es un turno y false si es idle.
// Los campos desconocidos se ignoran (comportamiento por defecto de encoding/json).
func ParseNext(data []byte) (Turn, bool, error) {
	var raw rawNext
	if err := json.Unmarshal(data, &raw); err != nil {
		return Turn{}, false, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	switch raw.Status {
	case "idle":
		return Turn{}, false, nil
	case "turn":
	default:
		return Turn{}, false, fmt.Errorf("%w: status %q", ErrMalformed, raw.Status)
	}

	if raw.GameID == "" {
		return Turn{}, false, fmt.Errorf("%w: game_id vacío", ErrMalformed)
	}
	if raw.You != 1 && raw.You != 2 {
		return Turn{}, false, fmt.Errorf("%w: you = %d", ErrMalformed, raw.You)
	}
	board, err := parseBoard(raw.Board)
	if err != nil {
		return Turn{}, false, err
	}
	moves, err := parseColumns("moves", raw.Moves)
	if err != nil {
		return Turn{}, false, err
	}
	legal, err := parseColumns("legal_moves", raw.LegalMoves)
	if err != nil {
		return Turn{}, false, err
	}
	if len(legal) == 0 {
		return Turn{}, false, fmt.Errorf("%w: legal_moves vacío", ErrMalformed)
	}

	turn := Turn{
		GameID:     raw.GameID,
		MatchID:    raw.MatchID,
		Kind:       raw.Kind,
		You:        game.Player(raw.You),
		Opponent:   raw.Opponent,
		Number:     raw.Turn,
		Board:      board,
		Moves:      moves,
		LegalMoves: legal,
		Deadline:   time.Duration(raw.DeadlineMS) * time.Millisecond,
		ServedAt:   raw.ServedAt,
	}
	if lm := raw.LastMove; lm != nil {
		if (lm.By != 1 && lm.By != 2) || lm.Column < 0 || lm.Column >= game.Cols {
			return Turn{}, false, fmt.Errorf("%w: last_move %+v", ErrMalformed, *lm)
		}
		turn.LastMove = &LastMove{By: game.Player(lm.By), Column: game.Column(lm.Column)}
	}
	return turn, true, nil
}

// parseBoard exige exactamente 7 filas de 8 casillas con valores 0, 1 o 2. No invierte filas:
// el árbitro y game.Board usan el mismo formato (board[fila][columna], fila 0 abajo).
func parseBoard(rows [][]int) (game.Board, error) {
	var b game.Board
	if len(rows) != game.Rows {
		return b, fmt.Errorf("%w: board tiene %d filas, se esperaban %d", ErrMalformed, len(rows), game.Rows)
	}
	for r, row := range rows {
		if len(row) != game.Cols {
			return b, fmt.Errorf("%w: board fila %d tiene %d columnas, se esperaban %d", ErrMalformed, r, len(row), game.Cols)
		}
		for c, v := range row {
			if v < 0 || v > 2 {
				return b, fmt.Errorf("%w: board[%d][%d] = %d", ErrMalformed, r, c, v)
			}
			b[r][c] = game.Cell(v)
		}
	}
	return b, nil
}

func parseColumns(field string, values []int) ([]game.Column, error) {
	cols := make([]game.Column, 0, len(values))
	for _, v := range values {
		if v < 0 || v >= game.Cols {
			return nil, fmt.Errorf("%w: %s contiene la columna %d", ErrMalformed, field, v)
		}
		cols = append(cols, game.Column(v))
	}
	return cols, nil
}

// MoveResult es la respuesta 200 de POST /api/move.
type MoveResult struct {
	State        string   // "playing" | "won" | "draw"
	Turn         int      // siguiente jugada, o la última si terminó
	WinningCells [][2]int // solo con State = "won"
}

func parseMove(data []byte) (MoveResult, error) {
	var raw struct {
		State        string   `json:"state"`
		Turn         int      `json:"turn"`
		WinningCells [][2]int `json:"winning_cells"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return MoveResult{}, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	switch raw.State {
	case "playing", "won", "draw":
	default:
		return MoveResult{}, fmt.Errorf("%w: state %q", ErrMalformed, raw.State)
	}
	return MoveResult{State: raw.State, Turn: raw.Turn, WinningCells: raw.WinningCells}, nil
}
