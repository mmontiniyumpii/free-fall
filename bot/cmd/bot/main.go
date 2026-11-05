// Command bot arranca el bot: warm-up del motor, el loop del árbitro (si ARBITRO_URL está definida)
// y la API local para la app web. En el torneo se corre bajo scripts/run-bot.sh (o .ps1), que lo reinicia si se cae.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/mmontiniyumpii/free-fall/bot/internal/api"
	"github.com/mmontiniyumpii/free-fall/bot/internal/arbiter"
	"github.com/mmontiniyumpii/free-fall/bot/internal/config"
	"github.com/mmontiniyumpii/free-fall/bot/internal/engine"
	"github.com/mmontiniyumpii/free-fall/bot/internal/game"
)

// engineFunc es la firma de engine.ChooseMove. Recibirla como parámetro permite probar
// el adaptador con un motor falso. El del árbitro vive en internal/arbiter (EngineChooser).
type engineFunc func(b game.Board, me game.Player, legal []game.Column, opts engine.Options) engine.Result

// localChooser adapta el motor a la API local, con la profundidad máxima de la dificultad.
func localChooser(choose engineFunc, think time.Duration) api.Chooser {
	return func(b game.Board, me game.Player, legal []game.Column, maxDepth int) api.BotMove {
		r := choose(b, me, legal, engine.Options{Budget: think, MaxDepth: maxDepth})
		return api.BotMove{Column: r.Column, Depth: r.Depth, Nodes: r.Nodes}
	}
}

// warmUp hace una búsqueda de d sobre el tablero vacío antes de jugar (plan §6), para que la primera
// jugada del torneo no pague el arranque en frío (código y memoria que todavía no están en caché).
func warmUp(choose engineFunc, d time.Duration) engine.Result {
	var empty game.Board
	return choose(empty, 1, game.LegalMoves(empty), engine.Options{Budget: d})
}

func main() {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		slog.Error("invalid config", "err", err)
		os.Exit(1)
	}

	// ctx se cancela con Ctrl+C o SIGTERM: el loop y el servidor terminan prolijamente.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, cfg, time.Second); err != nil {
		slog.Error("bot stopped", "err", err)
		os.Exit(1)
	}
}

// run arranca el bot con cfg y bloquea hasta que ctx se cancela. warm es la duración del warm-up.
// Devuelve error si el servidor no puede arrancar.
func run(ctx context.Context, cfg config.Config, warm time.Duration) error {
	// El puerto se abre ANTES del warm-up y del loop: si ya hay otra instancia del bot en esta máquina,
	// esta falla sin haber pedido ningún turno al árbitro (docs/TECNICO.md, D15).
	ln, err := net.Listen("tcp", ":"+cfg.Port)
	if err != nil {
		return fmt.Errorf("listen on port %s (is another bot already running?): %w", cfg.Port, err)
	}

	w := warmUp(engine.ChooseMove, warm)
	slog.Info("engine warm-up", "depth", w.Depth, "nodes", w.Nodes, "elapsed_ms", w.Elapsed.Milliseconds())

	// connected lo escribe el loop del árbitro y lo lee GET /health.
	// atomic.Bool se puede usar desde varias goroutines sin mutex.
	var connected atomic.Bool
	if cfg.ArbiterURL != "" {
		client := arbiter.NewClient(cfg.ArbiterURL, cfg.TeamToken)
		opts := arbiter.LoopOptions{Wait: cfg.PollWait, Connected: &connected}
		// go: el loop corre en su propia goroutine, en paralelo con la API local.
		go arbiter.RunLoop(ctx, client, arbiter.EngineChooser(cfg.Think), slog.Default(), opts)
	} else {
		slog.Warn("ARBITRO_URL vacío: solo API local, sin loop del árbitro")
	}

	srv := &http.Server{
		Handler: api.NewServer(localChooser(engine.ChooseMove, cfg.Think), cfg.Think, &connected).Handler(),
		// Corta clientes que no terminan de mandar los headers (evita sockets colgados).
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done() // bloquea hasta Ctrl+C
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("shutdown", "err", err)
		}
	}()

	slog.Info("bot listening", "config", cfg)
	// Serve usa el puerto ya abierto y lo cierra al terminar.
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
