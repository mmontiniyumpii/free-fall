// Command bot arranca el bot. En el paso 0 (spike) solo sirve GET /health;
// el loop del árbitro, la API local y el warm-up llegan en pasos siguientes.
package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"
)

// Valores provisorios hasta que exista internal/config (paso 1).
const (
	defaultPort = "3000"
	thinkMS     = 1500
)

// healthResponse es el cuerpo de GET /health definido en docs/api-local.md.
type healthResponse struct {
	OK               bool `json:"ok"`
	ArbiterConnected bool `json:"arbiter_connected"`
	ThinkMS          int  `json:"think_ms"`
}

// newMux arma las rutas. Se separa de main para poder probarlo con httptest.
func newMux() *http.ServeMux {
	mux := http.NewServeMux()
	// "GET /health": desde Go 1.22 el patrón incluye el método; otro método devuelve 405 solo.
	mux.HandleFunc("GET /health", handleHealth)
	return mux
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	body := healthResponse{OK: true, ArbiterConnected: false, ThinkMS: thinkMS}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("write health response", "err", err)
	}
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: newMux(),
		// Corta clientes que no terminan de mandar los headers (evita sockets colgados).
		ReadHeaderTimeout: 5 * time.Second,
	}

	slog.Info("bot listening", "port", port)
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}
