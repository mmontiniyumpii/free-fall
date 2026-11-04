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

// RunLoop pide turnos y juega hasta que ctx se cancela. Nunca devuelve por un error:
// lo loguea, espera retryDelay y vuelve a intentar. Después de idle vuelve a llamar sin esperar.
func RunLoop(ctx context.Context, client arbiterClient, choose ChooseFunc, logger *slog.Logger, opts LoopOptions) {
	for ctx.Err() == nil {
		err := runOnce(ctx, client, choose, logger, opts)
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

// runOnce es una iteración: un long poll y, si hay turno, una jugada.
// El recover convierte cualquier panic en un error, para que el loop siga vivo.
func runOnce(ctx context.Context, client arbiterClient, choose ChooseFunc, logger *slog.Logger, opts LoopOptions) (err error) {
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
	warnIfIncoherent(logger, turn)
	choice := safeChoose(logger, choose, turn)
	col := guard(logger, turn, choice.Column)
	thinkMS := time.Since(received).Milliseconds()

	res, err := client.Move(ctx, turn.GameID, col)
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
		"think_ms", thinkMS, "elapsed_ms", elapsedMS, "state", res.State)
	return nil
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
