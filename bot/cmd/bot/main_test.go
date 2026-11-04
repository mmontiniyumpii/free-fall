package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mmontiniyumpii/free-fall/bot/internal/arbiter"
	"github.com/mmontiniyumpii/free-fall/bot/internal/config"
	"github.com/mmontiniyumpii/free-fall/bot/internal/game"
)

func getHealth(t *testing.T, connected bool) map[string]any {
	t.Helper()
	var flag atomic.Bool
	flag.Store(connected)
	srv := httptest.NewServer(newMux(config.Config{Think: 1500 * time.Millisecond}, &flag))
	// defer: Close se ejecuta al salir de la función, pase lo que pase.
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
	// Se decodifica a un mapa genérico para detectar claves de más o de menos.
	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return got
}

// El cuerpo de GET /health debe coincidir exactamente con docs/api-local.md.
func TestHealth(t *testing.T) {
	want := map[string]any{"ok": true, "arbiter_connected": false, "think_ms": float64(1500)}
	if got := getHealth(t, false); !reflect.DeepEqual(got, want) {
		t.Errorf("body = %v, want %v", got, want)
	}
}

func TestHealthReportsArbiterConnection(t *testing.T) {
	if got := getHealth(t, true); got["arbiter_connected"] != true {
		t.Errorf("arbiter_connected = %v, want true", got["arbiter_connected"])
	}
}

func TestHealthRejectsOtherMethods(t *testing.T) {
	srv := httptest.NewServer(newMux(config.Config{}, &atomic.Bool{}))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/health", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusMethodNotAllowed)
	}
}

func TestRandomChoiceIsAlwaysLegal(t *testing.T) {
	turn := arbiter.Turn{LegalMoves: []game.Column{2, 5}}
	for range 200 {
		if c := randomChoice(turn).Column; !slices.Contains(turn.LegalMoves, c) {
			t.Fatalf("randomChoice = %d, legal_moves = %v", c, turn.LegalMoves)
		}
	}
}
