// Command bot arranca el bot: la API local para la app web (siempre) y el loop del árbitro (si ARBITRO_URL está definida).
// El warm-up del motor llega en el paso 8.
package main

import (
	"context"
	"errors"
	"log/slog"
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

func main() {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		slog.Error("invalid config", "err", err)
		os.Exit(1)
	}

	// ctx se cancela con Ctrl+C o SIGTERM: el loop y el servidor terminan prolijamente.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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
		Addr:    ":" + cfg.Port,
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
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}
