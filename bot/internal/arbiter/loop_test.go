package arbiter

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mmontiniyumpii/free-fall/bot/internal/game"
)

type reply struct {
	status int
	body   []byte
}

// fakeArbiter entrega las respuestas de /api/next en orden; cuando se le acaban,
// retiene la conexión como un long poll hasta que el cliente corta.
type fakeArbiter struct {
	mu         sync.Mutex
	nexts      []reply
	moveStatus []int // status de cada POST /api/move en orden; si se acaban, 200
	moves      []map[string]any
	nextCalls  int
}

func (f *fakeArbiter) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	switch r.URL.Path {
	case "/api/next":
		f.nextCalls++
		if len(f.nexts) == 0 {
			f.mu.Unlock()
			<-r.Context().Done()
			return
		}
		rep := f.nexts[0]
		f.nexts = f.nexts[1:]
		f.mu.Unlock()
		respond(rep.status, rep.body)(w, r)
	case "/api/move":
		var body map[string]any
		data, _ := io.ReadAll(r.Body)
		json.Unmarshal(data, &body)
		f.moves = append(f.moves, body)
		status := 200
		if len(f.moveStatus) > 0 {
			status, f.moveStatus = f.moveStatus[0], f.moveStatus[1:]
		}
		f.mu.Unlock()
		if status == 200 {
			respond(200, []byte(`{"ok":true,"state":"playing","turn":2}`))(w, r)
		} else {
			respond(status, []byte(`{"ok":false,"error":"not_your_turn","message":"x"}`))(w, r)
		}
	default:
		f.mu.Unlock()
		http.NotFound(w, r)
	}
}

func (f *fakeArbiter) movesSnapshot() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[string]any(nil), f.moves...)
}

// turnJSON arma un turno coherente jugando moves desde el tablero vacío.
func turnJSON(t *testing.T, gameID string, moves ...int) []byte {
	t.Helper()
	if moves == nil {
		moves = []int{} // en JSON, [] y no null
	}
	var b game.Board
	for i, c := range moves {
		var err error
		if b, _, err = game.Apply(b, game.Column(c), game.Player(i%2+1)); err != nil {
			t.Fatal(err)
		}
	}
	var legal []int
	for _, c := range game.LegalMoves(b) {
		legal = append(legal, int(c))
	}
	data, err := json.Marshal(map[string]any{
		"status": "turn", "game_id": gameID, "match_id": nil, "kind": "practice",
		"you": len(moves)%2 + 1, "opponent": "casa-basico", "turn": len(moves) + 1,
		"board": b, "moves": moves, "last_move": nil, "legal_moves": legal,
		"deadline_ms": 3000, "served_at": "2026-11-05T15:04:05.123Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// syncBuffer es un bytes.Buffer protegido por mutex: el logger escribe desde la goroutine del loop.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type harness struct {
	fake      *fakeArbiter
	logs      *syncBuffer
	connected *atomic.Bool
	cancel    context.CancelFunc
	done      chan struct{}
}

// start levanta el árbitro falso y corre RunLoop en una goroutine hasta que el test termina.
func start(t *testing.T, fake *fakeArbiter, choose ChooseFunc) *harness {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(fake.handler))
	h := &harness{fake: fake, logs: &syncBuffer{}, connected: &atomic.Bool{}, done: make(chan struct{})}
	var ctx context.Context
	ctx, h.cancel = context.WithCancel(context.Background())
	logger := slog.New(slog.NewTextHandler(h.logs, nil))
	go func() {
		defer close(h.done) // cerrar el canal avisa a quien espere en <-h.done
		RunLoop(ctx, NewClient(ts.URL, token), choose, logger, LoopOptions{Wait: time.Second, Connected: h.connected})
	}()
	t.Cleanup(func() {
		h.stop(t)
		ts.Close()
	})
	return h
}

func (h *harness) stop(t *testing.T) {
	h.cancel()
	select {
	case <-h.done:
	case <-time.After(3 * time.Second):
		t.Fatal("RunLoop no terminó después de cancelar el contexto")
	}
}

// waitMoves espera hasta que el árbitro falso haya recibido n jugadas.
func (h *harness) waitMoves(t *testing.T, n int) []map[string]any {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if m := h.fake.movesSnapshot(); len(m) >= n {
			return m
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("el loop mandó %d jugadas, se esperaban %d\nlogs:\n%s", len(h.fake.movesSnapshot()), n, h.logs.String())
	return nil
}

func firstLegal(t Turn) Choice { return Choice{Column: t.LegalMoves[0], Depth: 3, Nodes: 42} }

func TestLoopPlaysEachTurnWithItsGameID(t *testing.T) {
	fake := &fakeArbiter{nexts: []reply{
		{200, readFixture(t, "next-idle.json")},
		{200, turnJSON(t, "g_a")},
		{200, readFixture(t, "next-idle.json")},
		{200, turnJSON(t, "g_b", 0, 0, 0, 0, 0, 0, 0)}, // columna 0 llena: la primera legal es 1
	}}
	h := start(t, fake, firstLegal)

	moves := h.waitMoves(t, 2)
	if moves[0]["game_id"] != "g_a" || moves[0]["column"] != 0.0 {
		t.Errorf("jugada 1 = %v, want g_a columna 0", moves[0])
	}
	if moves[1]["game_id"] != "g_b" || moves[1]["column"] != 1.0 {
		t.Errorf("jugada 2 = %v, want g_b columna 1", moves[1])
	}
	h.stop(t)

	logs := h.logs.String()
	for _, want := range []string{"game_id=g_a", "column=0", "depth=3", "nodes=42", "elapsed_ms=", "game_id=g_b"} {
		if !strings.Contains(logs, want) {
			t.Errorf("los logs no contienen %q:\n%s", want, logs)
		}
	}
	if strings.Contains(logs, token) {
		t.Errorf("los logs exponen el token:\n%s", logs)
	}
	if !h.connected.Load() {
		t.Error("connected = false después de llamadas exitosas")
	}
}

func TestLoopGuardReplacesIllegalChoice(t *testing.T) {
	fake := &fakeArbiter{nexts: []reply{{200, turnJSON(t, "g", 0, 0, 0, 0, 0, 0, 0)}}}
	h := start(t, fake, func(Turn) Choice { return Choice{Column: 0} }) // columna 0 está llena

	if m := h.waitMoves(t, 1); m[0]["column"] != 1.0 {
		t.Errorf("columna enviada = %v, want 1 (primera legal)", m[0]["column"])
	}
	if logs := h.logs.String(); !strings.Contains(logs, "illegal") {
		t.Errorf("el guard no dejó registro:\n%s", logs)
	}
}

func TestLoopRecoversFromPanicInChoose(t *testing.T) {
	fake := &fakeArbiter{nexts: []reply{{200, turnJSON(t, "g_1")}, {200, turnJSON(t, "g_2")}}}
	h := start(t, fake, func(Turn) Choice { panic("bug en el motor") })

	moves := h.waitMoves(t, 2) // juega igual (primera legal) y el loop sigue vivo
	if moves[0]["game_id"] != "g_1" || moves[1]["game_id"] != "g_2" {
		t.Errorf("moves = %v", moves)
	}
	if logs := h.logs.String(); !strings.Contains(logs, "bug en el motor") {
		t.Errorf("el panic no quedó en los logs:\n%s", logs)
	}
}

func TestLoopRetriesAfterErrorWithDelay(t *testing.T) {
	fake := &fakeArbiter{nexts: []reply{
		{502, []byte("<html>Bad Gateway</html>")},
		{200, turnJSON(t, "g")},
	}}
	begin := time.Now()
	h := start(t, fake, firstLegal)
	h.waitMoves(t, 1)
	if elapsed := time.Since(begin); elapsed < 900*time.Millisecond {
		t.Errorf("reintentó en %v; después de un error debe esperar ~1 s", elapsed)
	}
	if logs := h.logs.String(); !strings.Contains(logs, "502") {
		t.Errorf("el error no quedó en los logs:\n%s", logs)
	}
}

func TestLoopDoesNotSleepAfterIdleOrNotYourTurn(t *testing.T) {
	fake := &fakeArbiter{
		nexts: []reply{
			{200, readFixture(t, "next-idle.json")},
			{200, readFixture(t, "next-idle.json")},
			{200, turnJSON(t, "g_1")},
			{200, turnJSON(t, "g_2")},
		},
		moveStatus: []int{409}, // la primera jugada llega tarde: 409 no penaliza, se sigue
	}
	begin := time.Now()
	h := start(t, fake, firstLegal)
	h.waitMoves(t, 2)
	if elapsed := time.Since(begin); elapsed > 700*time.Millisecond {
		t.Errorf("tardó %v: después de idle o 409 no debe dormir", elapsed)
	}
	if logs := h.logs.String(); !strings.Contains(logs, "not_your_turn") {
		t.Errorf("el 409 no quedó en los logs:\n%s", logs)
	}
}

func TestLoopConnectedFlag(t *testing.T) {
	fake := &fakeArbiter{nexts: []reply{{401, readFixture(t, "error-unauthorized.json")}}}
	h := start(t, fake, firstLegal)
	deadline := time.Now().Add(2 * time.Second)
	for {
		fake.mu.Lock()
		calls := fake.nextCalls
		fake.mu.Unlock()
		if calls >= 1 && strings.Contains(h.logs.String(), "unauthorized") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no registró el 401:\n%s", h.logs.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if h.connected.Load() {
		t.Error("connected = true después de un 401")
	}
}

func TestLoopWarnsWhenBoardDoesNotMatchMoves(t *testing.T) {
	// El ejemplo del OpenAPI tiene 6 fichas en board y 8 jugadas en moves: se juega igual, con aviso.
	fake := &fakeArbiter{nexts: []reply{{200, readFixture(t, "next-turn.json")}}}
	h := start(t, fake, firstLegal)
	h.waitMoves(t, 1)
	if logs := h.logs.String(); !strings.Contains(logs, "board does not match moves") {
		t.Errorf("no avisó la incoherencia:\n%s", logs)
	}
}

func TestLoopStopsOnCancel(t *testing.T) {
	h := start(t, &fakeArbiter{}, firstLegal) // sin respuestas: el long poll queda colgado
	time.Sleep(100 * time.Millisecond)
	begin := time.Now()
	h.stop(t)
	if elapsed := time.Since(begin); elapsed > time.Second {
		t.Errorf("cancelar tardó %v", elapsed)
	}
}
