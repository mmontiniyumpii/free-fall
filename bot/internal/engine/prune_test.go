package engine

import (
	"testing"
	"time"

	"github.com/mmontiniyumpii/free-fall/bot/internal/game"
)

// Poda de jugadas perdedoras dentro del árbol (docs/TECNICO.md, D16). Los tests llaman a negamax
// directamente, sin la red de seguridad de la raíz, y miran el valor y cuántos nodos visitó.

func searcherFor(b game.Board) searcher {
	return searcher{pos: newPosition(b), hardStop: time.Now().Add(time.Hour)}
}

// (1) Si el que mueve gana en una jugada, el nodo vale la victoria y no explora ningún hijo,
// aunque la jugada ganadora sea la última del orden (columna 7).
func TestNodeWithImmediateWinStopsAtOnce(t *testing.T) {
	s := searcherFor(draw(t, e, e, e, e, e, "2.......", "22..111."))
	score := s.negamax(4, 0, -infinity, infinity, 1)
	if score != winScore-1 {
		t.Errorf("valor %d, want %d (gana en la jugada 1)", score, winScore-1)
	}
	if s.nodes != 1 {
		t.Errorf("visitó %d nodos, want 1: con una victoria inmediata no hace falta explorar", s.nodes)
	}
}

// (2) Con dos casillas ganadoras jugables del rival no hay defensa: el nodo vale la derrota en la
// jugada 2 sin explorar.
func TestNodeFacingTwoThreatsIsLost(t *testing.T) {
	s := searcherFor(draw(t, e, e, e, e, e, ".111....", ".222...."))
	score := s.negamax(4, 0, -infinity, infinity, 1)
	if want := -(winScore - 2); score != want {
		t.Errorf("valor %d, want %d (el rival gana en la jugada 2)", score, want)
	}
	if s.nodes != 1 {
		t.Errorf("visitó %d nodos, want 1", s.nodes)
	}
}

// (2) Con una sola casilla ganadora del rival, la única jugada que no pierde es taparla:
// a profundidad 1 se visita el nodo y un solo hijo, no los ocho.
func TestNodeFacingOneThreatOnlyBlocks(t *testing.T) {
	b := draw(t, e, e, e, e, "2.......", "2.......", "2.1..1.1") // el 2 amenaza (3,0)
	s := searcherFor(b)
	s.negamax(1, 0, -infinity, infinity, 1)
	if s.nodes != 2 {
		t.Errorf("visitó %d nodos, want 2 (el nodo y el bloqueo en la columna 0)", s.nodes)
	}
	moves, n, lost := s.pos.candidates(1)
	if lost || n != 1 || moves[0] != 0 {
		t.Errorf("candidates = %v (n %d, lost %v), want solo la columna 0", moves[:n], n, lost)
	}
}

// (3) Las jugadas que dejan una casilla ganadora del rival justo encima van al final del orden.
// Acá el 2 completa la fila 1 en (1,3) o en (1,7) si el 1 juega en la columna 3 o en la 7.
// No se eliminan (puede no haber otras): siguen estando, al final.
func TestMovesUnderARivalWinGoLast(t *testing.T) {
	s := searcherFor(draw(t, e, e, e, e, e, "....222.", "11..121."))
	moves, n, lost := s.pos.candidates(1)
	want := []game.Column{4, 2, 5, 1, 6, 0, 3, 7}
	if lost || n != len(want) {
		t.Fatalf("candidates = %v (n %d, lost %v), want %v", moves[:n], n, lost, want)
	}
	for i := range want {
		if moves[i] != want[i] {
			t.Fatalf("orden %v, want %v (las que pierden, 3 y 7, al final)", moves[:n], want)
		}
	}
}
