// Command bot arranca el bot: el loop del árbitro (si ARBITRO_URL está definida) y GET /health.
// La API local para la app web y el warm-up del motor llegan en pasos siguientes.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/mmontiniyumpii/free-fall/bot/internal/arbiter"
	"github.com/mmontiniyumpii/free-fall/bot/internal/config"
)

// healthResponse es el cuerpo de GET /health definido en docs/api-local.md.
type healthResponse struct {
	OK               bool `json:"ok"`
	ArbiterConnected bool `json:"arbiter_connected"`
	ThinkMS          int  `json:"think_ms"`
}

// newMux arma las rutas. Se separa de main para poder probarlo con httptest.
// connected lo actualiza el loop del árbitro; atomic.Bool se puede leer y escribir desde varias goroutines sin mutex.
func newMux(cfg config.Config, connected *atomic.Bool) *http.ServeMux {
	mux := http.NewServeMux()
	// "GET /health": desde Go 1.22 el patrón incluye el método; otro método devuelve 405 solo.
	mux.HandleFunc("GET /health", healthHandler(cfg, connected))
	return mux
}

// healthHandler devuelve un handler que "recuerda" cfg y connected (una closure: la función captura las variables).
func healthHandler(cfg config.Config, connected *atomic.Bool) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body := healthResponse{OK: true, ArbiterConnected: connected.Load(), ThinkMS: int(cfg.Think / time.Millisecond)}
		if err := json.NewEncoder(w).Encode(body); err != nil {
			slog.Error("write health response", "err", err)
		}
	}
}

// randomChoice es el elector provisorio del paso 4: una columna legal al azar.
// Se reemplaza por engine.ChooseMove en el paso 6. rand.IntN global es seguro entre goroutines.
func randomChoice(t arbiter.Turn) arbiter.Choice {
	return arbiter.Choice{Column: t.LegalMoves[rand.IntN(len(t.LegalMoves))]}
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

	var connected atomic.Bool
	if cfg.ArbiterURL != "" {
		client := arbiter.NewClient(cfg.ArbiterURL, cfg.TeamToken)
		opts := arbiter.LoopOptions{Wait: cfg.PollWait, Connected: &connected}
		// go: el loop corre en su propia goroutine, en paralelo con el servidor HTTP.
		go arbiter.RunLoop(ctx, client, randomChoice, slog.Default(), opts)
	} else {
		slog.Warn("ARBITRO_URL vacío: solo API local, sin loop del árbitro")
	}

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: newMux(cfg, &connected),
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
