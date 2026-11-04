package main

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/mmontiniyumpii/free-fall/bot/internal/game"
)

// board arma un tablero desde un dibujo: primera línea = fila de arriba, '.' vacío.
func board(t *testing.T, lines ...string) game.Board {
	t.Helper()
	var b game.Board
	for i, line := range lines {
		line = strings.ReplaceAll(line, " ", "")
		for c, ch := range line {
			if ch != '.' {
				b[game.Rows-1-i][c] = game.Cell(ch - '0')
			}
		}
	}
	return b
}

const e = "........"

func TestHouseMove(t *testing.T) {
	tests := []struct {
		name  string
		kind  string
		board game.Board
		me    game.Player
		want  game.Column
	}{
		{
			// La columna 3 tiene una ficha abajo: la del 2 cae en la fila 1 y completa 4.
			name:  "basic wins when it can",
			kind:  houseBasic,
			board: board(t, e, e, e, e, e, "222.....", "1121...."),
			me:    2,
			want:  3,
		},
		{
			// El 2 gana en la columna 0; el 1 amenaza en la 3 y en la 7. Ganar va primero.
			name:  "basic prefers winning over blocking",
			kind:  houseBasic,
			board: board(t, e, e, e, e, "2.......", "2.......", "2...111."),
			me:    2,
			want:  0,
		},
		{
			name:  "basic blocks a horizontal threat",
			kind:  houseBasic,
			board: board(t, e, e, e, e, e, e, "111..22."),
			me:    2,
			want:  3,
		},
		{
			name:  "basic blocks a vertical threat",
			kind:  houseBasic,
			board: board(t, e, e, e, e, "1.......", "1.......", "1.22...."),
			me:    2,
			want:  0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rng := rand.New(rand.NewPCG(1, 2))
			if got := houseMove(tc.kind, tc.board, tc.me, rng); got != tc.want {
				t.Errorf("houseMove = %d, want %d", got, tc.want)
			}
		})
	}
}

// Los dos rivales de la casa juegan siempre una columna legal, aunque quede una sola.
func TestHouseMoveAlwaysLegal(t *testing.T) {
	onlyCol6 := board(t,
		"121212.1",
		"212121.2",
		"121212.1",
		"212121.2",
		"121212.1",
		"212121.2",
		"121212.1",
	)
	for _, kind := range []string{houseRandom, houseBasic} {
		rng := rand.New(rand.NewPCG(7, 7))
		for i := range 200 {
			if got := houseMove(kind, onlyCol6, 2, rng); got != 6 {
				t.Fatalf("%s iteración %d: columna %d, la única legal es 6", kind, i, got)
			}
		}

		// En partidas aleatorias completas, cada jugada de la casa está en LegalMoves.
		for range 50 {
			var b game.Board
			p := game.Player(1)
			for len(game.LegalMoves(b)) > 0 {
				c := houseMove(kind, b, p, rng)
				if !slices.Contains(game.LegalMoves(b), c) {
					t.Fatalf("%s jugó la columna %d, ilegal en %v", kind, c, b)
				}
				nb, row, err := game.Apply(b, c, p)
				if err != nil {
					t.Fatal(err)
				}
				b = nb
				if game.FindWin(b, row, c) != nil {
					break
				}
				p = 3 - p
			}
		}
	}
}
