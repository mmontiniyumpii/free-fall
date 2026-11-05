package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mmontiniyumpii/free-fall/bot/internal/config"
	"github.com/mmontiniyumpii/free-fall/bot/internal/engine"
	"github.com/mmontiniyumpii/free-fall/bot/internal/game"
)

// Los tests de GET /health y de la API local viven en internal/api.

// fakeEngine registra con qué opciones lo llamaron y devuelve una jugada fija.
type fakeEngine struct {
	opts []engine.Options
}

func (f *fakeEngine) choose(_ game.Board, _ game.Player, legal []game.Column, opts engine.Options) engine.Result {
	f.opts = append(f.opts, opts)
	return engine.Result{Column: legal[len(legal)-1], Depth: 9, Nodes: 1234}
}

// El test de que el árbitro juega siempre con MaxDepth = 0 vive en internal/arbiter (chooser_test.go).

// El warm-up busca sobre el tablero vacío durante el tiempo pedido, sin límite de profundidad (plan §6).
func TestWarmUp(t *testing.T) {
	var f fakeEngine
	r := warmUp(f.choose, time.Second)
	if len(f.opts) != 1 || f.opts[0] != (engine.Options{Budget: time.Second, MaxDepth: 0}) {
		t.Errorf("opciones = %+v, want Budget 1s y MaxDepth 0", f.opts)
	}
	if r.Depth != 9 {
		t.Errorf("warmUp devolvió %+v, want el resultado del motor", r)
	}
}

func TestWarmUpWithRealEngine(t *testing.T) {
	start := time.Now()
	r := warmUp(engine.ChooseMove, 100*time.Millisecond)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("el warm-up de 100 ms tardó %v", elapsed)
	}
	if r.Depth < 1 || r.Nodes < 1 {
		t.Errorf("warm-up sin búsqueda: %+v", r)
	}
}

func TestLocalChooserPassesDifficultyDepth(t *testing.T) {
	var f fakeEngine
	choose := localChooser(f.choose, 1500*time.Millisecond)
	for _, maxDepth := range []int{2, 6, 0} {
		choose(game.Board{}, 1, []game.Column{3}, maxDepth)
	}
	for i, want := range []int{2, 6, 0} {
		if f.opts[i] != (engine.Options{Budget: 1500 * time.Millisecond, MaxDepth: want}) {
			t.Errorf("llamada %d: opciones = %+v, want MaxDepth %d", i, f.opts[i], want)
		}
	}
}

// Si el puerto ya está ocupado (otra instancia del bot en la misma máquina), el arranque falla
// sin haberle hablado al árbitro: ni un GET /api/next que pueda llevarse un turno.
func TestBusyPortFailsBeforeTalkingToTheArbiter(t *testing.T) {
	busy, err := net.Listen("tcp", ":0") // :0 = el sistema elige un puerto libre; queda ocupado hasta Close
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	port := strconv.Itoa(busy.Addr().(*net.TCPAddr).Port)

	var requests atomic.Int32
	arb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Write([]byte(`{"status":"idle"}`))
	}))
	defer arb.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := config.Config{ArbiterURL: arb.URL, TeamToken: "tok", Think: 100 * time.Millisecond, Port: port, PollWait: 0}
	if err := run(ctx, cfg, 10*time.Millisecond); err == nil {
		t.Fatal("run con el puerto ocupado no devolvió error")
	}
	time.Sleep(300 * time.Millisecond) // margen para que un loop ya lanzado llegue a pedir turno
	if n := requests.Load(); n != 0 {
		t.Errorf("el árbitro recibió %d requests de una instancia que no pudo abrir su puerto; want 0", n)
	}
}

// Con el puerto libre, run sirve la API local y termina sin error al cancelar el contexto.
func TestRunServesHealthAndStopsOnCancel(t *testing.T) {
	free, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(free.Addr().(*net.TCPAddr).Port)
	free.Close() // se libera para que run lo abra

	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() {
		errc <- run(ctx, config.Config{Think: 100 * time.Millisecond, Port: port}, 10*time.Millisecond)
	}()

	var resp *http.Response
	for range 50 {
		if resp, err = http.Get("http://127.0.0.1:" + port + "/health"); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /health = %d", resp.StatusCode)
	}
	cancel()
	select {
	case err := <-errc:
		if err != nil {
			t.Errorf("run terminó con error al cancelar: %v", err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("run no terminó al cancelar el contexto")
	}
}
