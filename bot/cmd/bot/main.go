// Command bot arranca el bot. Por ahora solo sirve GET /health;
// el loop del árbitro, la API local y el warm-up llegan en pasos siguientes.
package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/mmontiniyumpii/free-fall/bot/internal/config"
)

// healthResponse es el cuerpo de GET /health definido en docs/api-local.md.
type healthResponse struct {
	OK               bool `json:"ok"`
	ArbiterConnected bool `json:"arbiter_connected"`
	ThinkMS          int  `json:"think_ms"`
}

// newMux arma las rutas. Se separa de main para poder probarlo con httptest.
func newMux(cfg config.Config) *http.ServeMux {
	mux := http.NewServeMux()
	// "GET /health": desde Go 1.22 el patrón incluye el método; otro método devuelve 405 solo.
	mux.HandleFunc("GET /health", healthHandler(cfg))
	return mux
}

// healthHandler devuelve un handler que "recuerda" cfg (una closure: la función captura la variable).
func healthHandler(cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// arbiter_connected queda en false hasta que exista el loop del árbitro (paso 4).
		body := healthResponse{OK: true, ArbiterConnected: false, ThinkMS: int(cfg.Think / time.Millisecond)}
		if err := json.NewEncoder(w).Encode(body); err != nil {
			slog.Error("write health response", "err", err)
		}
	}
}

func main() {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		slog.Error("invalid config", "err", err)
		os.Exit(1)
	}

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: newMux(cfg),
		// Corta clientes que no terminan de mandar los headers (evita sockets colgados).
		ReadHeaderTimeout: 5 * time.Second,
	}

	slog.Info("bot listening", "config", cfg)
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}
