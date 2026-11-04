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
