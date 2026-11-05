package arbiter

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync/atomic"
	"time"

	"github.com/mmontiniyumpii/free-fall/bot/internal/game"
)

// Choice es la columna elegida y los datos de la búsqueda que la produjo (para el log).
type Choice struct {
	Column game.Column
	Depth  int
	Nodes  int64
}

// ChooseFunc elige la jugada de un turno. En el paso 6 es engine.ChooseMove con MaxDepth = 0.
type ChooseFunc func(Turn) Choice

// arbiterClient es lo que el loop necesita del cliente. *Client lo cumple.
type arbiterClient interface {
	Next(ctx context.Context, wait time.Duration) (Turn, bool, error)
	Move(ctx context.Context, gameID string, col game.Column) (MoveResult, error)
}

// LoopOptions agrupa la configuración del loop.
type LoopOptions struct {
	Wait      time.Duration // POLL_WAIT
	Connected *atomic.Bool  // si no es nil, refleja si la última llamada al árbitro funcionó (para /health)
}

// retryDelay es la espera después de un error, para no martillar al árbitro.
const retryDelay = time.Second

// moveDeadline es el límite propio para que la jugada llegue, contado desde la recepción del turno.
// El árbitro da 3 000 ms desde served_at; los 400 ms restantes cubren la red de ida y el reloj del árbitro.
const moveDeadline = 2600 * time.Millisecond

// Entre reintentos del POST se espera retryPause, para no girar en vacío si la conexión se rechaza al instante.
// Cada intento tiene al menos minAttempt, aunque el límite ya haya pasado (turno re-entregado: ver decision).
const (
	retryPause = 50 * time.Millisecond
	minAttempt = 300 * time.Millisecond
)

// decision es la última jugada elegida. Si el árbitro vuelve a entregar el mismo turno (porque el POST
// no llegó), se juega la misma columna sin volver a pensar, y el límite se sigue contando desde la
// primera recepción. Vive dentro de RunLoop: no es estado de paquete.
type decision struct {
	gameID   string
	number   int // turn.Number: número de turno dentro de la partida
	choice   Choice
	column   game.Column
	received time.Time
}

// RunLoop pide turnos y juega hasta que ctx se cancela. Nunca devuelve por un error:
// lo loguea, espera retryDelay y vuelve a intentar. Después de idle vuelve a llamar sin esperar.
func RunLoop(ctx context.Context, client arbiterClient, choose ChooseFunc, logger *slog.Logger, opts LoopOptions) {
	var last decision
	for ctx.Err() == nil {
		err := runOnce(ctx, client, choose, logger, opts, &last)
		if err == nil || ctx.Err() != nil {
			continue
		}
		if opts.Connected != nil {
			opts.Connected.Store(false)
		}
		logger.Error("arbiter loop", "err", err)
		// Espera retryDelay, pero corta antes si se cancela el contexto.
		select {
		case <-time.After(retryDelay):
		case <-ctx.Done():
		}
	}
}

// runOnce es una iteración: un long poll y, si hay turno, una jugada. last es la última decisión,
// que runOnce lee y actualiza (por eso es un puntero).
// El recover convierte cualquier panic en un error, para que el loop siga vivo.
func runOnce(ctx context.Context, client arbiterClient, choose ChooseFunc, logger *slog.Logger, opts LoopOptions, last *decision) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()

	turn, ok, err := client.Next(ctx, opts.Wait)
	if err != nil {
		return err
	}
	if opts.Connected != nil {
		opts.Connected.Store(true)
	}
	if !ok {
		return nil // idle
	}

	received := time.Now() // el tiempo se mide desde que llega el turno, nunca desde served_at
	if last.gameID == turn.GameID && last.number == turn.Number {
		// El mismo turno otra vez: el POST anterior no llegó. Se juega la columna ya elegida.
		logger.Warn("turn delivered again, replaying remembered move", "game_id", turn.GameID, "column", last.column,
			"since_first_ms", time.Since(last.received).Milliseconds())
	} else {
		warnIfIncoherent(logger, turn)
		choice := safeChoose(logger, choose, turn)
		*last = decision{gameID: turn.GameID, number: turn.Number, choice: choice,
			column: guard(logger, turn, choice.Column), received: received}
	}
	received = last.received
	choice, col := last.choice, last.column
	thinkMS := time.Since(received).Milliseconds()

	res, attempts, err := moveWithRetry(ctx, client, logger, turn.GameID, col, received.Add(moveDeadline))
	elapsedMS := time.Since(received).Milliseconds()
	var apiErr *APIError
	if errors.As(err, &apiErr) && (apiErr.Status == 400 || apiErr.Status == 409) {
		// 409: el turno ya no estaba pendiente (no penaliza). 400: jugada rechazada (la partida se perdió).
		// En los dos casos no hay nada que reintentar: se sigue con el próximo turno sin esperar.
		logger.Warn("move rejected", "game_id", turn.GameID, "column", col, "status", apiErr.Status,
			"error", apiErr.Code, "message", apiErr.Message, "elapsed_ms", elapsedMS)
		return nil
	}
	if err != nil {
		return err
	}
	logger.Info("turn", "game_id", turn.GameID, "column", col, "depth", choice.Depth, "nodes", choice.Nodes,
		"think_ms", thinkMS, "elapsed_ms", elapsedMS, "attempts", attempts, "state", res.State)
	return nil
}

// moveWithRetry envía la jugada y, si falla por la red, reenvía la MISMA columna hasta deadline.
// Es seguro: si un envío anterior sí llegó, el árbitro responde 409 not_your_turn, que no penaliza.
// Un 400 o un 409 son respuestas del árbitro y no se reintentan. Devuelve cuántos intentos hizo.
func moveWithRetry(ctx context.Context, client arbiterClient, logger *slog.Logger, gameID string, col game.Column, deadline time.Time) (MoveResult, int, error) {
	for attempt := 1; ; attempt++ {
		// Cada intento vence en el límite (o en minAttempt, si queda menos): mejor que los 5 s fijos del cliente,
		// que superan el plazo del árbitro. WithTimeout devuelve un contexto hijo que se cancela solo al vencer.
		attemptCtx, cancel := context.WithTimeout(ctx, max(time.Until(deadline), minAttempt))
		res, err := client.Move(attemptCtx, gameID, col)
		cancel() // libera el temporizador del contexto apenas termina el intento
		var apiErr *APIError
		if err == nil || ctx.Err() != nil || (errors.As(err, &apiErr) && (apiErr.Status == 400 || apiErr.Status == 409)) {
			return res, attempt, err
		}
		if time.Until(deadline) <= retryPause {
			return res, attempt, fmt.Errorf("move not delivered after %d attempts: %w", attempt, err)
		}
		logger.Warn("move failed, retrying same column", "game_id", gameID, "column", col, "attempt", attempt, "err", err)
		select {
		case <-time.After(retryPause):
		case <-ctx.Done():
			return res, attempt, ctx.Err()
		}
	}
}

// safeChoose llama a choose; si entra en panic, juega la primera columna legal en vez de perder el turno.
func safeChoose(logger *slog.Logger, choose ChooseFunc, turn Turn) (c Choice) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("choose panicked, playing first legal move", "game_id", turn.GameID, "panic", fmt.Sprint(r))
			c = Choice{Column: turn.LegalMoves[0]}
		}
	}()
	return choose(turn)
}

// guard garantiza que la columna enviada esté en legal_moves (ParseNext asegura que no está vacío).
func guard(logger *slog.Logger, turn Turn, col game.Column) game.Column {
	if slices.Contains(turn.LegalMoves, col) {
		return col
	}
	logger.Error("illegal choice replaced by first legal move", "game_id", turn.GameID, "column", col,
		"legal_moves", fmt.Sprint(turn.LegalMoves))
	return turn.LegalMoves[0]
}

// warnIfIncoherent avisa si board no coincide con el tablero reconstruido desde moves.
// No es un error: el motor usa board, que es lo que el árbitro valida.
func warnIfIncoherent(logger *slog.Logger, turn Turn) {
	var b game.Board
	for i, c := range turn.Moves {
		next, _, err := game.Apply(b, c, game.Player(i%2+1))
		if err != nil {
			break
		}
		b = next
	}
	if b != turn.Board {
		logger.Warn("board does not match moves", "game_id", turn.GameID, "moves", len(turn.Moves))
	}
}
