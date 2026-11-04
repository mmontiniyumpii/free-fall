package main

import (
	"slices"
	"testing"

	"github.com/mmontiniyumpii/free-fall/bot/internal/arbiter"
	"github.com/mmontiniyumpii/free-fall/bot/internal/game"
)

// Los tests de GET /health y de la API local viven en internal/api.

func TestRandomChoosersAreAlwaysLegal(t *testing.T) {
	legal := []game.Column{2, 5}
	for range 200 {
		if c := randomChoice(arbiter.Turn{LegalMoves: legal}).Column; !slices.Contains(legal, c) {
			t.Fatalf("randomChoice = %d, legal_moves = %v", c, legal)
		}
		if c := randomBotMove(game.Board{}, 1, legal, 0).Column; !slices.Contains(legal, c) {
			t.Fatalf("randomBotMove = %d, legal_moves = %v", c, legal)
		}
	}
}
