// Package engine elige la jugada del bot en tiempo acotado: negamax con poda alfa-beta,
// iterative deepening, orden de jugadas desde el centro y heurística por ventanas (PLAN_BACKEND.md §5).
// No conoce HTTP ni al árbitro: recibe un game.Board y devuelve una columna.
package engine
