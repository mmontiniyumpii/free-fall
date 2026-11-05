//go:build audit

package arbiter

// Escenarios de falla de la revisión (docs/revision-bot.md). Solo con -tags audit:
//
//	go test -tags audit -run Audit -v -timeout 5m ./internal/arbiter

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Un medio juego real (16 fichas): el motor usa todo el presupuesto. Es la posición fija de
// move_retry_test.go, que antes se generaba acá jugando motor contra motor en cada corrida.
func auditTurn(t *testing.T) []byte {
	t.Helper()
	return midgameTurn(t)
}

type timeline struct {
	mu     sync.Mutex
	events []string
	served time.Time // primera vez que se entregó el turno (desde acá corre el reloj del árbitro)
	moved  time.Time // jugada aceptada
}

func (tl *timeline) add(format string, start time.Time) {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	tl.events = append(tl.events, time.Since(start).Round(10*time.Millisecond).String()+" "+format)
}

// Respuesta lenta (25 s), JSON roto, 401, turno; el primer POST se corta a mitad de camino.
func TestAuditLoopSurvivesHostileArbiter(t *testing.T) {
	start := time.Now()
	turn := auditTurn(t)
	var nexts, posts atomic.Int32
	tl := &timeline{}
	done := make(chan struct{})
	var once sync.Once

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/next":
			n := nexts.Add(1)
			switch {
			case n == 1:
				tl.add("next #1: respuesta lenta de 25 s (idle)", start)
				select {
				case <-time.After(25 * time.Second):
				case <-r.Context().Done():
					return
				}
				w.Write([]byte(`{"status":"idle"}`))
			case n == 2:
				tl.add("next #2: JSON malformado", start)
				w.Write([]byte(`{"status":"turn","board":[[0,0`))
			case n == 3:
				tl.add("next #3: 401", start)
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"ok":false,"error":"unauthorized","message":"Token inválido."}`))
			default:
				tl.mu.Lock()
				if tl.served.IsZero() {
					tl.served = time.Now()
				}
				tl.mu.Unlock()
				tl.add("next: turno g_audit", start)
				w.Write(turn)
			}
		case "/api/move":
			body, _ := io.ReadAll(r.Body)
			if posts.Add(1) == 1 {
				tl.add("move #1: conexión cortada antes de responder "+string(body), start)
				conn, _, err := w.(http.Hijacker).Hijack()
				if err == nil {
					conn.Close()
				}
				return
			}
			tl.mu.Lock()
			tl.moved = time.Now()
			tl.mu.Unlock()
			tl.add("move #2: aceptada "+string(body), start)
			w.Write([]byte(`{"ok":true,"state":"playing","turn":18}`))
			once.Do(func() { close(done) })
		}
	}))
	defer ts.Close()

	var logs bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go RunLoop(ctx, NewClient(ts.URL, token), EngineChooser(1500*time.Millisecond),
		slog.New(slog.NewTextHandler(&logs, nil)), LoopOptions{Wait: 20 * time.Second})

	select {
	case <-done:
	case <-time.After(90 * time.Second):
		t.Fatal("el loop nunca envió la jugada")
	}
	cancel()
	for _, e := range tl.events {
		t.Log(e)
	}
	total := tl.moved.Sub(tl.served)
	t.Logf("desde la primera entrega del turno hasta la jugada aceptada: %v (límite del árbitro 3 s)", total.Round(time.Millisecond))
	if strings.Contains(logs.String(), token) {
		t.Error("el token apareció en los logs")
	}
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		t.Log("log: ", line)
	}
	if total > 3*time.Second {
		t.Errorf("un POST cortado una sola vez hace perder la partida por timeout: %v > 3 s", total.Round(time.Millisecond))
	}
}

// TestAuditReceptionToMoveUnder1800ms pasó a la suite normal: TestReceptionToMoveUnder1800ms (move_retry_test.go).

// El servidor se cae (conexión rechazada) y vuelve en la misma dirección: el loop se recupera solo.
func TestAuditLoopRecoversWhenServerComesBack(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close() // caído: nadie escucha

	var logs bytes.Buffer
	var mu sync.Mutex
	logger := slog.New(slog.NewTextHandler(lockedWriter{&mu, &logs}, nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	moved := make(chan struct{})
	var once sync.Once
	go RunLoop(ctx, NewClient("http://"+addr, token), EngineChooser(100*time.Millisecond), logger, LoopOptions{Wait: time.Second})

	time.Sleep(3 * time.Second)
	ln2, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/next" {
			w.Write(auditTurn(t))
			return
		}
		w.Write([]byte(`{"ok":true,"state":"playing","turn":18}`))
		once.Do(func() { close(moved) })
	}))
	ts.Listener.Close()
	ts.Listener = ln2
	ts.Start()
	defer ts.Close()
	select {
	case <-moved:
	case <-time.After(10 * time.Second):
		t.Fatal("no se recuperó al volver el servidor")
	}
	mu.Lock()
	t.Logf("errores logueados mientras estuvo caído: %d", strings.Count(logs.String(), "arbiter loop"))
	mu.Unlock()
}

type lockedWriter struct {
	mu *sync.Mutex
	w  io.Writer
}

func (l lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}
