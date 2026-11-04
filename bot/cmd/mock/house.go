package main

import (
	"math/rand/v2"

	"github.com/mmontiniyumpii/free-fall/bot/internal/game"
)

// Rivales de la casa, con los mismos nombres que PracticeRequest.opponent del OpenAPI.
const (
	houseRandom = "casa-aleatorio" // juega al azar
	houseBasic  = "casa-basico"    // gana y bloquea cuatros inmediatos
)

// houseMove elige la jugada de la casa. Solo se llama con la partida en curso, así que hay al menos una columna legal.
func houseMove(kind string, b game.Board, me game.Player, rng *rand.Rand) game.Column {
	legal := game.LegalMoves(b)
	if kind == houseBasic {
		if c, ok := winningMove(b, me, legal); ok {
			return c
		}
		if c, ok := winningMove(b, 3-me, legal); ok { // 3-me es el rival: 3-1 = 2, 3-2 = 1
			return c
		}
	}
	return legal[rng.IntN(len(legal))]
}

// winningMove busca una columna con la que p completa cuatro en línea.
func winningMove(b game.Board, p game.Player, legal []game.Column) (game.Column, bool) {
	for _, c := range legal {
		nb, row, err := game.Apply(b, c, p)
		if err == nil && game.FindWin(nb, row, c) != nil {
			return c, true
		}
	}
	return 0, false
}
