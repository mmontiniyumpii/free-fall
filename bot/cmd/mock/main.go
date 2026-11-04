// Command mock es el árbitro simulado, fiel a docs/bases/arbitro-openapi.json, para desarrollar sin el árbitro real.
//
// Uso: go run ./cmd/mock [-addr :4000] [-seed N]
// Cualquier token Bearer no vacío es un equipo. MOCK_LATENCY_MS agrega una demora a cada respuesta.
package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/mmontiniyumpii/free-fall/bot/internal/config"
)

func main() {
	addr := flag.String("addr", ":4000", "dirección donde escucha el mock")
	seed := flag.Uint64("seed", 0, "semilla de los rivales de la casa (0 = según la hora)")
	flag.Parse() // flag.String devuelve un puntero: *addr es el valor después de Parse

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		slog.Error("invalid config", "err", err)
		os.Exit(1)
	}
	if *seed == 0 {
		*seed = uint64(time.Now().UnixNano())
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           newServer(cfg.MockLatency, *seed, time.Now).routes(),
		ReadHeaderTimeout: 5 * time.Second,
		// Sin WriteTimeout: el long poll retiene la conexión hasta 25 s.
	}
	slog.Info("mock arbiter listening", "addr", *addr, "latency", cfg.MockLatency, "seed", *seed)
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}
