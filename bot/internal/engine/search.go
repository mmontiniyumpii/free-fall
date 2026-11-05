package engine

import (
	"time"

	"github.com/mmontiniyumpii/free-fall/bot/internal/game"
)

const (
	infinity = 1 << 30
	// winScore es el valor de ganar. Se le resta el ply (jugadas desde la raíz) para preferir
	// la victoria más rápida y, si se pierde, la derrota más lenta.
	winScore = 100_000
	// checkEvery: el reloj se consulta cada 2 048 nodos; time.Now() en cada nodo sería demasiado caro.
	checkEvery = 2048
)

// searcher guarda el estado de UNA búsqueda. ChooseMove crea uno nuevo en cada llamada:
// no hay estado compartido, por eso el motor se puede llamar desde varias goroutines a la vez.
type searcher struct {
	pos      position
	nodes    int64
	hardStop time.Time // si se pasa, se aborta la profundidad en curso
	stopped  bool
}

// negamax devuelve el valor de la posición para who, mirando depth jugadas hacia adelante.
// Negamax usa que "lo bueno para mí es lo malo para el rival": el valor de una jugada es
// el negativo del valor que obtiene el rival desde la posición resultante.
// Poda alfa-beta: alpha es lo mínimo que who ya tiene asegurado; beta, lo máximo que el rival
// le va a permitir. Si una jugada alcanza beta, el rival nunca dejaría llegar acá: se corta (poda).
func (s *searcher) negamax(depth, ply, alpha, beta int, who uint8) int {
	s.nodes++
	if s.nodes%checkEvery == 0 && time.Now().After(s.hardStop) {
		s.stopped = true
	}
	if s.stopped {
		return 0 // el valor no importa: la profundidad abortada se descarta entera
	}
	if s.pos.moves == game.Cells {
		return 0 // tablero lleno sin ganador: empate
	}
	if depth == 0 {
		return evaluate(&s.pos, who)
	}

	// Poda de jugadas perdedoras (D16). Si who gana en una jugada, ese es el valor del nodo:
	// no hace falta explorar el resto.
	for _, c := range centerFirst {
		if s.pos.canPlay(c) && s.pos.winsWith(c, who) {
			return winScore - (ply + 1)
		}
	}
	// Si el rival tiene dos casillas ganadoras jugables, who pierde en la jugada siguiente a la suya.
	moves, n, lost := s.pos.candidates(who)
	if lost {
		return -(winScore - (ply + 2))
	}

	best := -infinity
	for _, c := range moves[:n] {
		// Ningún hijo gana en el acto (se descartó arriba), así que no hace falta mirar wins después de drop.
		s.pos.drop(c, who)
		score := -s.negamax(depth-1, ply+1, -beta, -alpha, 3-who)
		s.pos.undo(c)
		if s.stopped {
			return 0
		}
		if score > best {
			best = score
		}
		if best > alpha {
			alpha = best
		}
		if alpha >= beta {
			break // poda: el rival no va a permitir esta línea
		}
	}
	return best
}

// searchRoot busca una profundidad completa desde la raíz y devuelve la mejor columna.
// ok = false si se abortó por tiempo: en ese caso el resultado no sirve y se usa el de la profundidad anterior.
func (s *searcher) searchRoot(depth int, me uint8, order [game.Cols]game.Column, n int) (best game.Column, bestScore int, ok bool) {
	best, bestScore = order[0], -infinity
	alpha := -infinity
	for i := range n {
		c := order[i]
		s.pos.drop(c, me)
		var score int
		if s.pos.wins(c) {
			score = winScore - 1
		} else {
			score = -s.negamax(depth-1, 1, -infinity, -alpha, 3-me)
		}
		s.pos.undo(c)
		if s.stopped {
			return best, bestScore, false
		}
		if score > bestScore {
			best, bestScore = c, score
		}
		if score > alpha {
			alpha = score
		}
	}
	return best, bestScore, true
}
