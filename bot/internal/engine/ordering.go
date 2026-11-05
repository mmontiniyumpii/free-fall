package engine

import "github.com/mmontiniyumpii/free-fall/bot/internal/game"

// centerFirst es el orden en que se prueban las jugadas: del centro a los bordes.
// Las columnas del centro participan en más líneas, así que suelen ser mejores; probar primero
// las mejores hace que alfa-beta pode antes. Es un array (valor fijo), no un slice: no se modifica.
var centerFirst = [game.Cols]game.Column{3, 4, 2, 5, 1, 6, 0, 7}

// rootOrder devuelve las columnas a probar en la raíz: primero la mejor de la iteración anterior (pv),
// después el resto en orden centerFirst. Solo incluye columnas de legal que el tablero permite.
// Devuelve un array y su largo para no pedir memoria.
func rootOrder(p *position, legal []game.Column, pv game.Column, hasPV bool) ([game.Cols]game.Column, int) {
	var order [game.Cols]game.Column
	n := 0
	allowed := func(c game.Column) bool {
		if int(c) >= game.Cols || !p.canPlay(c) {
			return false
		}
		for _, l := range legal {
			if l == c {
				return true
			}
		}
		return false
	}
	if hasPV && allowed(pv) {
		order[n] = pv
		n++
	}
	for _, c := range centerFirst {
		if (!hasPV || c != pv) && allowed(c) {
			order[n] = c
			n++
		}
	}
	return order, n
}

// candidates devuelve las jugadas de who que vale la pena explorar en un nodo interior, ya sabiendo
// que who no gana en una jugada (eso lo mira negamax antes). Es la poda de jugadas perdedoras (D16):
//
//   - si el rival tiene dos o más casillas ganadoras jugables, no hay defensa: lost = true;
//   - si tiene una, la única jugada que no pierde enseguida es taparla;
//   - si no tiene ninguna, todas, del centro a los bordes, pero las que le dejan al rival una casilla
//     ganadora justo encima van al final. No se eliminan: puede no haber otras.
//
// Devuelve un array y su largo, como rootOrder, para no pedir memoria.
func (p *position) candidates(who uint8) (moves [game.Cols]game.Column, n int, lost bool) {
	rival := 3 - who
	threats := 0
	for _, c := range centerFirst {
		if p.canPlay(c) && p.winsWith(c, rival) {
			threats++
			moves[0] = c
		}
	}
	switch {
	case threats >= 2:
		return moves, 0, true
	case threats == 1:
		return moves, 1, false
	}

	var losing [game.Cols]game.Column // las que dejan una casilla ganadora del rival encima, en orden
	nLosing := 0
	for _, c := range centerFirst {
		if !p.canPlay(c) {
			continue
		}
		p.drop(c, who)
		givesWin := p.canPlay(c) && p.winsWith(c, rival)
		p.undo(c)
		if givesWin {
			losing[nLosing] = c
			nLosing++
		} else {
			moves[n] = c
			n++
		}
	}
	for i := range nLosing {
		moves[n] = losing[i]
		n++
	}
	return moves, n, false
}
