package arbiter

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mmontiniyumpii/free-fall/bot/internal/game"
)

// midgameMoves es un medio juego real de 16 fichas: dos motores a profundidad 6 desde [3 4 2]
// (la misma posición que usa la revisión, docs/revision-bot.md). El motor gasta todo el presupuesto en ella.
var midgameMoves = []int{3, 4, 2, 3, 3, 4, 2, 2, 3, 1, 3, 3, 4, 4, 2, 6}

// midgameTurn es la respuesta de GET /api/next para esa posición, con un campo desconocido de yapa.
func midgameTurn(t *testing.T) []byte {
	t.Helper()
	var b game.Board
	for i, c := range midgameMoves {
		var err error
		if b, _, err = game.Apply(b, game.Column(c), game.Player(i%2+1)); err != nil {
			t.Fatal(err)
		}
	}
	// []game.Column es []uint8 y encoding/json lo serializaría como base64: se pasa a []int.
	var legal []int
	for _, c := range game.LegalMoves(b) {
		legal = append(legal, int(c))
	}
	data, err := json.Marshal(map[string]any{
		"status": "turn", "game_id": "g_mid", "match_id": nil, "kind": "league", "you": 1 + len(midgameMoves)%2,
		"opponent": "rival", "turn": len(midgameMoves) + 1, "board": b, "moves": midgameMoves,
		"last_move": map[string]int{"by": 2, "column": 6}, "legal_moves": legal, "deadline_ms": 3000,
		"served_at": "2026-11-05T15:04:05.123Z", "campo_nuevo": "ignorado",
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// cutArbiter simula un árbitro que entrega el turno de midgameTurn y corta algunos POST /api/move.
type cutArbiter struct {
	turn       []byte
	deliveries int                                // cuántas veces entrega el turno; después retiene el long poll
	cut        func(post int, now time.Time) bool // true = cortar la conexión del POST número post (desde 1)

	mu        sync.Mutex
	nextCalls int
	served    time.Time // primera entrega del turno: desde acá corre el reloj del árbitro
	posts     []time.Time
	columns   []float64
	accepted  time.Time
	done      chan struct{}
	closeOnce sync.Once
}

func newCutArbiter(t *testing.T, deliveries int, cut func(post int, now time.Time) bool) *cutArbiter {
	return &cutArbiter{turn: midgameTurn(t), deliveries: deliveries, cut: cut, done: make(chan struct{})}
}

func (a *cutArbiter) handler(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	switch r.URL.Path {
	case "/api/next":
		a.mu.Lock()
		a.nextCalls++
		deliver := a.nextCalls <= a.deliveries
		if deliver && a.served.IsZero() {
			a.served = now
		}
		a.mu.Unlock()
		if !deliver {
			<-r.Context().Done() // long poll sin turnos hasta que el cliente corta
			return
		}
		w.Write(a.turn)
	case "/api/move":
		var body struct {
			Column float64 `json:"column"`
		}
		data, _ := io.ReadAll(r.Body)
		json.Unmarshal(data, &body)
		a.mu.Lock()
		a.posts = append(a.posts, now)
		a.columns = append(a.columns, body.Column)
		cut := a.cut(len(a.posts), now)
		if !cut {
			a.accepted = now
		}
		a.mu.Unlock()
		if cut {
			// Hijack toma la conexión TCP de manos del servidor HTTP; cerrarla sin responder es un corte de red.
			if conn, _, err := w.(http.Hijacker).Hijack(); err == nil {
				conn.Close()
			}
			return
		}
		w.Write([]byte(`{"ok":true,"state":"playing","turn":18}`))
		a.closeOnce.Do(func() { close(a.done) })
	}
}

// snapshot copia el estado bajo el mutex para leerlo sin carreras.
func (a *cutArbiter) snapshot() (served time.Time, posts []time.Time, columns []float64, accepted time.Time, nextCalls int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.served, append([]time.Time(nil), a.posts...), append([]float64(nil), a.columns...), a.accepted, a.nextCalls
}

// slowChooser tarda think y juega la primera columna legal; cuenta cuántas veces lo llaman.
func slowChooser(think time.Duration, calls *atomic.Int32) ChooseFunc {
	return func(t Turn) Choice {
		calls.Add(1)
		time.Sleep(think)
		return Choice{Column: t.LegalMoves[0], Depth: 1}
	}
}

func startLoop(t *testing.T, a *cutArbiter, choose ChooseFunc) context.CancelFunc {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(a.handler))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel(); ts.Close() })
	go RunLoop(ctx, NewClient(ts.URL, token), choose, slog.New(slog.DiscardHandler), LoopOptions{Wait: time.Second})
	return cancel
}

// (a) El primer POST se corta: la misma columna se reenvía enseguida y llega antes de los 3 s del árbitro,
// aunque el motor haya pensado 1 750 ms.
func TestMoveIsRetriedAfterACutConnection(t *testing.T) {
	t.Parallel()
	a := newCutArbiter(t, 1, func(post int, _ time.Time) bool { return post == 1 })
	var calls atomic.Int32
	startLoop(t, a, slowChooser(1750*time.Millisecond, &calls))

	select {
	case <-a.done:
	case <-time.After(10 * time.Second):
		t.Fatal("la jugada nunca fue aceptada")
	}
	served, posts, columns, accepted, _ := a.snapshot()
	if d := accepted.Sub(served); d >= 3*time.Second {
		t.Errorf("jugada aceptada %v después de la primera entrega: el árbitro la daría por timeout (límite 3 s)", d.Round(time.Millisecond))
	}
	if len(posts) != 2 || columns[0] != columns[1] {
		t.Errorf("POSTs = %d con columnas %v; want 2 con la misma columna", len(posts), columns)
	}
	if calls.Load() != 1 {
		t.Errorf("el motor pensó %d veces; want 1 (el reintento reusa la columna)", calls.Load())
	}
}

// (b) Si todos los POST fallan, deja de reintentar al llegar al límite de 2 600 ms desde la recepción
// y el loop sigue vivo: vuelve a pedir turno.
func TestMoveRetriesStopAtTheDeadlineAndTheLoopSurvives(t *testing.T) {
	t.Parallel()
	a := newCutArbiter(t, 1, func(int, time.Time) bool { return true })
	var calls atomic.Int32
	startLoop(t, a, slowChooser(0, &calls))

	deadline := time.Now().Add(8 * time.Second)
	for {
		if _, _, _, _, nextCalls := a.snapshot(); nextCalls >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("después de agotar los reintentos, el loop no volvió a pedir turno")
		}
		time.Sleep(20 * time.Millisecond)
	}
	served, posts, _, _, _ := a.snapshot()
	if len(posts) < 2 {
		t.Fatalf("hubo %d POST; want al menos un reintento", len(posts))
	}
	// Margen para el último intento, que arranca antes del límite y tarda lo que tarde el corte.
	if last := posts[len(posts)-1].Sub(served); last > moveDeadline+200*time.Millisecond {
		t.Errorf("último POST a %v de la entrega: no debe reintentar pasado el límite de %v", last.Round(time.Millisecond), moveDeadline)
	}
}

// (c) Si los POST fallan hasta que el árbitro vuelve a entregar el mismo turno, se juega la columna
// guardada sin volver a llamar al motor.
func TestRedeliveredTurnReusesTheDecision(t *testing.T) {
	t.Parallel()
	var a *cutArbiter
	a = newCutArbiter(t, 2, func(int, time.Time) bool {
		return a.nextCalls < 2 // se llama con a.mu tomado: corta todo hasta la segunda entrega
	})
	var calls atomic.Int32
	startLoop(t, a, slowChooser(10*time.Millisecond, &calls))

	select {
	case <-a.done:
	case <-time.After(10 * time.Second):
		t.Fatal("la jugada nunca fue aceptada")
	}
	_, _, columns, _, nextCalls := a.snapshot()
	if nextCalls < 2 {
		t.Fatalf("el turno se entregó %d veces; el test necesita 2", nextCalls)
	}
	if calls.Load() != 1 {
		t.Errorf("el motor pensó %d veces; want 1 (el turno re-entregado reusa la decisión)", calls.Load())
	}
	for _, c := range columns {
		if c != columns[0] {
			t.Errorf("columnas enviadas %v; want siempre la misma", columns)
			break
		}
	}
}

// Desde que llega el turno hasta que sale el POST, con el motor real: la parada dura (1 750 ms) más
// el envío tiene que quedar por debajo de 1 800 ms. Con -race el detector frena el motor (race_on_test.go).
func TestReceptionToMoveUnder1800ms(t *testing.T) {
	a := newCutArbiter(t, 1, func(int, time.Time) bool { return false })
	startLoop(t, a, EngineChooser(1500*time.Millisecond))

	select {
	case <-a.done:
	case <-time.After(10 * time.Second):
		t.Fatal("la jugada nunca fue aceptada")
	}
	served, _, _, accepted, _ := a.snapshot()
	limit := 1800 * time.Millisecond
	if raceEnabled {
		limit += 150 * time.Millisecond
	}
	d := accepted.Sub(served)
	t.Logf("turno entregado → POST recibido: %v", d.Round(time.Millisecond))
	if d > limit {
		t.Errorf("%v > %v", d.Round(time.Millisecond), limit)
	}
}
