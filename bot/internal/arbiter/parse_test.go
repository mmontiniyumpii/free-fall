package arbiter

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/mmontiniyumpii/free-fall/bot/internal/game"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// withField devuelve el fixture de turno con una clave reemplazada (o borrada si value es remove).
var remove = struct{}{}

func withField(t *testing.T, key string, value any) []byte {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(readFixture(t, "next-turn.json"), &m); err != nil {
		t.Fatal(err)
	}
	if value == remove {
		delete(m, key)
	} else {
		m[key] = value
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestParseNextTurnFixture(t *testing.T) {
	turn, ok, err := ParseNext(readFixture(t, "next-turn.json"))
	if err != nil || !ok {
		t.Fatalf("ParseNext = ok %v, err %v", ok, err)
	}
	if turn.GameID != "g_7f3a" || turn.MatchID == nil || *turn.MatchID != "m_12" || turn.Kind != "league" ||
		turn.You != 2 || turn.Opponent != "Los Recursivos" || turn.Number != 9 ||
		turn.Deadline != 3000*time.Millisecond || turn.ServedAt != "2026-11-05T15:04:05.123Z" {
		t.Errorf("campos inesperados: %+v", turn)
	}
	if turn.LastMove == nil || *turn.LastMove != (LastMove{By: 1, Column: 3}) {
		t.Errorf("last_move = %+v", turn.LastMove)
	}
	if want := []game.Column{3, 2, 3, 2, 4, 3, 2, 3}; !reflect.DeepEqual(turn.Moves, want) {
		t.Errorf("moves = %v, want %v", turn.Moves, want)
	}
	if want := []game.Column{0, 1, 2, 3, 4, 5, 6, 7}; !reflect.DeepEqual(turn.LegalMoves, want) {
		t.Errorf("legal_moves = %v, want %v", turn.LegalMoves, want)
	}
}

// board[fila][columna] con fila 0 abajo: la primera fila del JSON es la de abajo, sin invertir nada.
func TestParseNextBoardOrientation(t *testing.T) {
	turn, _, err := ParseNext(readFixture(t, "next-turn.json"))
	if err != nil {
		t.Fatal(err)
	}
	// Fixture: fila 0 = [0,0,1,2,1,0,0,0], fila 1 = [0,0,2,1,0,...], fila 2 = [0,0,0,2,0,...].
	checks := []struct {
		row, col int
		want     game.Cell
	}{
		{0, 2, 1}, {0, 3, 2}, {0, 4, 1}, {1, 2, 2}, {1, 3, 1}, {2, 3, 2}, {6, 3, 0}, {0, 0, 0},
	}
	for _, c := range checks {
		if got := turn.Board[c.row][c.col]; got != c.want {
			t.Errorf("board[%d][%d] = %d, want %d", c.row, c.col, got, c.want)
		}
	}
}

func TestParseNextIdle(t *testing.T) {
	turn, ok, err := ParseNext(readFixture(t, "next-idle.json"))
	if err != nil || ok {
		t.Fatalf("ParseNext(idle) = ok %v, err %v; want false, nil", ok, err)
	}
	if !reflect.DeepEqual(turn, Turn{}) {
		t.Errorf("turno no vacío en idle: %+v", turn)
	}
}

func TestParseNextNullsAndUnknownFields(t *testing.T) {
	var m map[string]any
	if err := json.Unmarshal(readFixture(t, "next-turn.json"), &m); err != nil {
		t.Fatal(err)
	}
	m["match_id"] = nil
	m["last_move"] = nil
	m["campo_nuevo"] = map[string]any{"x": 1} // campos que agregue el árbitro definitivo se ignoran
	data, _ := json.Marshal(m)

	turn, ok, err := ParseNext(data)
	if err != nil || !ok {
		t.Fatalf("ParseNext = ok %v, err %v", ok, err)
	}
	if turn.MatchID != nil || turn.LastMove != nil {
		t.Errorf("match_id = %v, last_move = %v; want nil, nil", turn.MatchID, turn.LastMove)
	}
}

func TestParseNextInvalid(t *testing.T) {
	row := []int{0, 0, 0, 0, 0, 0, 0, 0}
	sixRows := [][]int{row, row, row, row, row, row}
	shortRow := [][]int{{0, 0, 0, 0, 0, 0, 0}, row, row, row, row, row, row}
	badCell := [][]int{{0, 0, 3, 0, 0, 0, 0, 0}, row, row, row, row, row, row}

	tests := []struct {
		name string
		data []byte
	}{
		{"not json", []byte("<html>")},
		{"unknown status", withField(t, "status", "paused")},
		{"missing status", withField(t, "status", remove)},
		{"empty game_id", withField(t, "game_id", "")},
		{"you out of range", withField(t, "you", 3)},
		{"board with 6 rows", withField(t, "board", sixRows)},
		{"board row with 7 columns", withField(t, "board", shortRow)},
		{"board cell with value 3", withField(t, "board", badCell)},
		{"missing board", withField(t, "board", remove)},
		{"empty legal_moves", withField(t, "legal_moves", []int{})},
		{"legal move out of range", withField(t, "legal_moves", []int{0, 8})},
		{"move out of range", withField(t, "moves", []int{3, 9})},
		{"last_move with invalid player", withField(t, "last_move", map[string]int{"by": 0, "column": 3})},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, ok, err := ParseNext(tc.data)
			if err == nil || ok {
				t.Fatalf("ParseNext = ok %v, err %v; want error", ok, err)
			}
			if !errors.Is(err, ErrMalformed) {
				t.Errorf("err = %v, want ErrMalformed", err)
			}
		})
	}
}

func TestParseMove(t *testing.T) {
	tests := []struct {
		file string
		want MoveResult
	}{
		{"move-playing.json", MoveResult{State: "playing", Turn: 10}},
		{"move-won.json", MoveResult{State: "won", Turn: 10, WinningCells: [][2]int{{0, 2}, {1, 3}, {2, 4}, {3, 5}}}},
		{"move-draw.json", MoveResult{State: "draw", Turn: 56}},
	}
	for _, tc := range tests {
		t.Run(tc.file, func(t *testing.T) {
			got, err := parseMove(readFixture(t, tc.file))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseMove = %+v, want %+v", got, tc.want)
			}
		})
	}
	if _, err := parseMove([]byte(`{"ok":true,"state":"raro"}`)); !errors.Is(err, ErrMalformed) {
		t.Errorf("state desconocido: err = %v, want ErrMalformed", err)
	}
}
