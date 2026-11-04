package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mmontiniyumpii/free-fall/bot/internal/game"
)

// recorder es un elector de prueba: juega la primera columna legal (o la preferida, si es legal)
// y guarda con qué maxDepth lo llamaron.
type recorder struct {
	mu        sync.Mutex
	depths    []int
	prefer    int // -1: primera legal
	calls     int
	blockOnce chan struct{} // si no es nil, la primera llamada espera hasta que se cierre
}

func newRecorder() *recorder { return &recorder{prefer: -1} }

func (r *recorder) choose(b game.Board, me game.Player, legal []game.Column, maxDepth int) BotMove {
	r.mu.Lock()
	r.depths = append(r.depths, maxDepth)
	r.calls++
	block := r.blockOnce
	r.blockOnce = nil
	prefer := r.prefer
	r.mu.Unlock()
	if block != nil {
		<-block
	}
	col := legal[0]
	if prefer >= 0 && slices.Contains(legal, game.Column(prefer)) {
		col = game.Column(prefer)
	}
	return BotMove{Column: col, Depth: maxDepth + 1, Nodes: 7}
}

func (r *recorder) snapshot() (calls int, depths []int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls, append([]int(nil), r.depths...)
}

func newTestAPI(t *testing.T, choose Chooser) (*Server, *httptest.Server) {
	t.Helper()
	var connected atomic.Bool
	s := NewServer(choose, 1500*time.Millisecond, &connected)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return s, ts
}

func call(t *testing.T, ts *httptest.Server, method, path, body string) (int, map[string]any) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, ts.URL+path, r)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if ct := resp.Header.Get("Content-Type"); len(data) > 0 && ct != "application/json" {
		t.Errorf("%s %s: Content-Type = %q", method, path, ct)
	}
	var m map[string]any
	if len(data) > 0 {
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("%s %s: respuesta no es JSON: %s", method, path, data)
		}
	}
	return resp.StatusCode, m
}

func create(t *testing.T, ts *httptest.Server, body string) map[string]any {
	t.Helper()
	status, st := call(t, ts, "POST", "/local/games", body)
	if status != http.StatusCreated {
		t.Fatalf("POST /local/games %s: status %d %v", body, status, st)
	}
	return st
}

func play(t *testing.T, ts *httptest.Server, id string, col int) (int, map[string]any) {
	t.Helper()
	return call(t, ts, "POST", "/local/games/"+id+"/moves", fmt.Sprintf(`{"column":%d}`, col))
}

func contractFixture(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile("../../../docs/fixtures/game-state.example.json")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// sameShape exige exactamente las mismas claves y los mismos tipos JSON que want, recursivamente.
// Si want trae null, got puede traer cualquier tipo; las claves en nullable pueden venir null en got.
func sameShape(t *testing.T, got, want map[string]any, nullable ...string) {
	t.Helper()
	for k := range want {
		if _, ok := got[k]; !ok {
			t.Errorf("falta la clave %q", k)
		}
	}
	for k, g := range got {
		w, ok := want[k]
		if !ok {
			t.Errorf("clave %q que no está en el contrato", k)
			continue
		}
		if w == nil || (g == nil && slices.Contains(nullable, k)) {
			continue
		}
		if reflect.TypeOf(g) != reflect.TypeOf(w) {
			t.Errorf("%q: tipo %T, el contrato tiene %T", k, g, w)
			continue
		}
		if wm, ok := w.(map[string]any); ok {
			sameShape(t, g.(map[string]any), wm)
		}
		if wa, ok := w.([]any); ok {
			ga := g.([]any)
			if len(wa) > 0 && len(ga) > 0 && reflect.TypeOf(ga[0]) != reflect.TypeOf(wa[0]) {
				t.Errorf("%q[]: tipo %T, el contrato tiene %T", k, ga[0], wa[0])
			}
		}
	}
}

func ints(v any) []int {
	var out []int
	for _, x := range v.([]any) {
		out = append(out, int(x.(float64)))
	}
	return out
}

func boardOf(t *testing.T, st map[string]any) game.Board {
	t.Helper()
	data, _ := json.Marshal(st["board"])
	var b game.Board
	if err := json.Unmarshal(data, &b); err != nil {
		t.Fatal(err)
	}
	return b
}

func rebuild(t *testing.T, history []int) game.Board {
	t.Helper()
	var b game.Board
	for i, c := range history {
		var err error
		if b, _, err = game.Apply(b, game.Column(c), game.Player(i%2+1)); err != nil {
			t.Fatalf("history[%d] = %d: %v", i, c, err)
		}
	}
	return b
}

func TestCreateBotFirstMatchesFixture(t *testing.T) {
	_, ts := newTestAPI(t, newRecorder().choose)
	st := create(t, ts, `{"first":"bot","difficulty":"hard"}`)
	sameShape(t, st, contractFixture(t))

	if !strings.HasPrefix(st["id"].(string), "l_") || st["status"] != "playing" || st["difficulty"] != "hard" {
		t.Errorf("estado inesperado: %v", st)
	}
	if st["human"] != 2.0 || st["bot"] != 1.0 || st["to_move"] != 2.0 {
		t.Errorf("human/bot/to_move = %v/%v/%v, want 2/1/2", st["human"], st["bot"], st["to_move"])
	}
	history := ints(st["history"])
	if len(history) != 1 || boardOf(t, st) != rebuild(t, history) {
		t.Errorf("la apertura del bot no es coherente: history %v", history)
	}
	lb := st["last_bot"].(map[string]any)
	if lb["column"] != float64(history[0]) {
		t.Errorf("last_bot.column = %v, want %d", lb["column"], history[0])
	}
}

func TestCreateHumanFirst(t *testing.T) {
	rec := newRecorder()
	_, ts := newTestAPI(t, rec.choose)
	st := create(t, ts, `{"first":"human"}`)
	sameShape(t, st, contractFixture(t), "last_bot")

	if st["human"] != 1.0 || st["bot"] != 2.0 || st["to_move"] != 1.0 || st["last_bot"] != nil {
		t.Errorf("estado inesperado: %v", st)
	}
	if h, ok := st["history"].([]any); !ok || len(h) != 0 {
		t.Errorf("history = %#v, want [] (no null)", st["history"])
	}
	if boardOf(t, st) != (game.Board{}) || !reflect.DeepEqual(ints(st["legal_moves"]), []int{0, 1, 2, 3, 4, 5, 6, 7}) {
		t.Errorf("tablero o legal_moves inesperados: %v", st)
	}
	if calls, _ := rec.snapshot(); calls != 0 {
		t.Errorf("el bot jugó %d veces en una partida que abre el humano", calls)
	}
}

func TestDifficultyMapsToMaxDepth(t *testing.T) {
	tests := []struct {
		body      string
		wantLevel string
		wantDepth int
	}{
		{`{"first":"bot"}`, "hard", 0},
		{`{"first":"bot","difficulty":null}`, "hard", 0},
		{`{"first":"bot","difficulty":"easy"}`, "easy", 2},
		{`{"first":"bot","difficulty":"medium"}`, "medium", 6},
		{`{"first":"bot","difficulty":"hard"}`, "hard", 0},
	}
	for _, tc := range tests {
		t.Run(tc.body, func(t *testing.T) {
			rec := newRecorder()
			_, ts := newTestAPI(t, rec.choose)
			st := create(t, ts, tc.body)
			if st["difficulty"] != tc.wantLevel {
				t.Errorf("difficulty = %v, want %s", st["difficulty"], tc.wantLevel)
			}
			if _, depths := rec.snapshot(); !reflect.DeepEqual(depths, []int{tc.wantDepth}) {
				t.Errorf("maxDepth = %v, want [%d]", depths, tc.wantDepth)
			}
			// La dificultad se mantiene en las jugadas siguientes.
			play(t, ts, st["id"].(string), 0)
			if _, depths := rec.snapshot(); len(depths) != 2 || depths[1] != tc.wantDepth {
				t.Errorf("maxDepth en la segunda jugada = %v, want %d", depths, tc.wantDepth)
			}
		})
	}
}

func TestCreateValidation(t *testing.T) {
	_, ts := newTestAPI(t, newRecorder().choose)
	for _, body := range []string{
		``, `not json`, `{}`, `{"first":"alien"}`, `{"first":"human","difficulty":"imposible"}`,
		`{"first":"human","difficulty":5}`, `{"first":1}`,
	} {
		status, st := call(t, ts, "POST", "/local/games", body)
		if status != http.StatusBadRequest || st["ok"] != false || st["error"] != "validation" || st["message"] == "" {
			t.Errorf("body %q: %d %v, want 400 validation", body, status, st)
		}
	}
}

func TestMoveBotRepliesInSameCall(t *testing.T) {
	rec := newRecorder()
	_, ts := newTestAPI(t, rec.choose)
	id := create(t, ts, `{"first":"human","difficulty":"medium"}`)["id"].(string)

	status, st := play(t, ts, id, 3)
	if status != http.StatusOK {
		t.Fatalf("status %d: %v", status, st)
	}
	sameShape(t, st, contractFixture(t))
	history := ints(st["history"])
	if len(history) != 2 || history[0] != 3 {
		t.Fatalf("history = %v, want [3, jugada del bot]", history)
	}
	if boardOf(t, st) != rebuild(t, history) {
		t.Error("board no coincide con history")
	}
	lb := st["last_bot"].(map[string]any)
	if lb["column"] != float64(history[1]) || lb["depth"] != 7.0 || lb["nodes"] != 7.0 {
		t.Errorf("last_bot = %v", lb)
	}
	if st["to_move"] != 1.0 || st["status"] != "playing" {
		t.Errorf("to_move/status = %v/%v", st["to_move"], st["status"])
	}

	// GET devuelve el mismo estado.
	if status, got := call(t, ts, "GET", "/local/games/"+id, ""); status != http.StatusOK || !reflect.DeepEqual(got, st) {
		t.Errorf("GET = %d %v\nwant %v", status, got, st)
	}
}

func TestMoveErrors(t *testing.T) {
	s, ts := newTestAPI(t, newRecorder().choose)
	id := create(t, ts, `{"first":"human"}`)["id"].(string)

	expect := func(name string, status int, st map[string]any, wantStatus int, wantErr string) {
		t.Helper()
		if status != wantStatus || st["ok"] != false || st["error"] != wantErr || st["message"] == "" {
			t.Errorf("%s: %d %v, want %d %s", name, status, st, wantStatus, wantErr)
		}
	}

	st, body := play(t, ts, "l_9999", 0)
	expect("partida inexistente", st, body, 404, "unknown_game")
	st, body = call(t, ts, "GET", "/local/games/l_9999", "")
	expect("GET inexistente", st, body, 404, "unknown_game")

	for _, b := range []string{`not json`, `{}`, `{"column":"3"}`, `{"column":2.5}`} {
		st, body = call(t, ts, "POST", "/local/games/"+id+"/moves", b)
		expect("body "+b, st, body, 400, "validation")
	}
	st, body = play(t, ts, id, 8)
	expect("columna 8", st, body, 400, "illegal_move")
	st, body = play(t, ts, id, -1)
	expect("columna -1", st, body, 400, "illegal_move")

	// Humano y bot (primera legal) apilan en la columna 0 hasta llenarla.
	for range 4 {
		if st, body := play(t, ts, id, 0); st != 200 {
			t.Fatalf("apilando en la columna 0: %d %v", st, body)
		}
	}
	st, body = play(t, ts, id, 0)
	expect("columna llena", st, body, 400, "illegal_move")
	if body["message"] != "La columna 0 está llena." {
		t.Errorf("message = %q, want el del contrato", body["message"])
	}

	// Defensa: si por algún motivo no le toca al humano.
	s.mu.Lock()
	g := s.games[id]
	s.mu.Unlock()
	g.mu.Lock()
	g.toMove = g.bot
	g.mu.Unlock()
	st, body = play(t, ts, id, 1)
	expect("no es su turno", st, body, 409, "not_your_turn")
}

// playUntilEnd juega como humano la primera columna legal hasta que la partida termina.
func playUntilEnd(t *testing.T, ts *httptest.Server, st map[string]any) map[string]any {
	t.Helper()
	id := st["id"].(string)
	for st["status"] == "playing" {
		legal := ints(st["legal_moves"])
		var status int
		status, st = play(t, ts, id, legal[0])
		if status != http.StatusOK {
			t.Fatalf("move: %d %v", status, st)
		}
	}
	return st
}

func TestFullGamesAreConsistent(t *testing.T) {
	for _, first := range []string{"human", "bot"} {
		t.Run(first, func(t *testing.T) {
			_, ts := newTestAPI(t, newRecorder().choose)
			st := playUntilEnd(t, ts, create(t, ts, `{"first":"`+first+`"}`))
			sameShape(t, st, contractFixture(t), "to_move")

			history := ints(st["history"])
			b := boardOf(t, st)
			if b != rebuild(t, history) {
				t.Error("board final no coincide con history")
			}
			if st["to_move"] != nil || len(st["legal_moves"].([]any)) != 0 {
				t.Errorf("partida terminada con to_move %v y legal_moves %v", st["to_move"], st["legal_moves"])
			}
			switch st["status"] {
			case "won":
				winner := game.Cell(st["winner"].(float64))
				cells := st["winning_cells"].([]any)
				if len(cells) != 4 {
					t.Fatalf("winning_cells = %v", cells)
				}
				for _, c := range cells {
					rc := c.([]any)
					if b[int(rc[0].(float64))][int(rc[1].(float64))] != winner {
						t.Errorf("celda %v no es del ganador %d", rc, winner)
					}
				}
			case "draw":
				if st["winner"] != nil || st["winning_cells"] != nil || len(history) != game.Cells {
					t.Errorf("empate incoherente: %v", st)
				}
			default:
				t.Errorf("status = %v", st["status"])
			}

			_, again := play(t, ts, st["id"].(string), 0)
			if again["error"] != "game_over" {
				t.Errorf("jugar en partida terminada: %v, want game_over", again)
			}
		})
	}
}

func TestHumanWinEndsWithoutBotMove(t *testing.T) {
	rec := newRecorder()
	rec.prefer = 7 // el bot apila en la columna 7 y no bloquea
	_, ts := newTestAPI(t, rec.choose)
	id := create(t, ts, `{"first":"human"}`)["id"].(string)

	var st map[string]any
	for col := range 4 {
		_, st = play(t, ts, id, col)
	}
	if st["status"] != "won" || st["winner"] != 1.0 || st["to_move"] != nil {
		t.Fatalf("estado = %v", st)
	}
	want := []any{[]any{0.0, 0.0}, []any{0.0, 1.0}, []any{0.0, 2.0}, []any{0.0, 3.0}}
	if !reflect.DeepEqual(st["winning_cells"], want) {
		t.Errorf("winning_cells = %v, want %v", st["winning_cells"], want)
	}
	if h := ints(st["history"]); !reflect.DeepEqual(h, []int{0, 7, 1, 7, 2, 7, 3}) {
		t.Errorf("history = %v: el bot no debe jugar después de que gana el humano", h)
	}
	if calls, _ := rec.snapshot(); calls != 3 {
		t.Errorf("el bot jugó %d veces, want 3", calls)
	}
}

func TestHumanDraw(t *testing.T) {
	s, ts := newTestAPI(t, newRecorder().choose)
	id := create(t, ts, `{"first":"bot"}`)["id"].(string) // humano = jugador 2

	// Tablero lleno sin cuatro en línea, menos la casilla (6,7), que completa el humano (2).
	rows := []string{"21222122", "12112111", "12221121", "12111222", "21221112", "11212211", "2112212."}
	s.mu.Lock()
	g := s.games[id]
	s.mu.Unlock()
	g.mu.Lock()
	g.board = game.Board{}
	g.history = g.history[:0]
	for r, line := range rows {
		for c, ch := range line {
			if ch != '.' {
				g.board[r][c] = game.Cell(ch - '0')
				g.history = append(g.history, game.Column(c))
			}
		}
	}
	g.toMove = g.human
	g.mu.Unlock()

	status, st := play(t, ts, id, 7)
	if status != http.StatusOK || st["status"] != "draw" || st["winner"] != nil || st["winning_cells"] != nil || st["to_move"] != nil {
		t.Fatalf("empate: %d %v", status, st)
	}
	if l, ok := st["legal_moves"].([]any); !ok || len(l) != 0 {
		t.Errorf("legal_moves = %#v, want []", st["legal_moves"])
	}
}

func TestBotGuardAndPanic(t *testing.T) {
	full := func(game.Board, game.Player, []game.Column, int) BotMove { return BotMove{Column: 9} }
	_, ts := newTestAPI(t, full)
	st := create(t, ts, `{"first":"bot"}`)
	if h := ints(st["history"]); h[0] != 0 {
		t.Errorf("con elección ilegal el bot debía jugar la primera legal (0), jugó %v", h)
	}

	panicky := func(game.Board, game.Player, []game.Column, int) BotMove { panic("bug en el motor") }
	_, ts = newTestAPI(t, panicky)
	status, st := call(t, ts, "POST", "/local/games", `{"first":"bot"}`)
	if status != http.StatusCreated || ints(st["history"])[0] != 0 {
		t.Errorf("con panic en el motor: %d %v, want 201 y la primera legal", status, st)
	}
}

func TestCORS(t *testing.T) {
	_, ts := newTestAPI(t, newRecorder().choose)
	req, _ := http.NewRequest("OPTIONS", ts.URL+"/local/games", nil)
	req.Header.Set("Origin", "http://localhost:4200")
	req.Header.Set("Access-Control-Request-Method", "POST")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("preflight status = %d, want 204", resp.StatusCode)
	}
	h := resp.Header
	if h.Get("Access-Control-Allow-Origin") != "*" || !strings.Contains(h.Get("Access-Control-Allow-Methods"), "POST") ||
		!strings.Contains(h.Get("Access-Control-Allow-Headers"), "Content-Type") {
		t.Errorf("headers CORS = %v", h)
	}

	resp, err = http.Post(ts.URL+"/local/games", "application/json", strings.NewReader(`{"first":"human"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Error("las respuestas normales también necesitan Access-Control-Allow-Origin")
	}
}

func TestHealth(t *testing.T) {
	var connected atomic.Bool
	s := NewServer(newRecorder().choose, 1500*time.Millisecond, &connected)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	status, st := call(t, ts, "GET", "/health", "")
	want := map[string]any{"ok": true, "arbiter_connected": false, "think_ms": 1500.0}
	if status != http.StatusOK || !reflect.DeepEqual(st, want) {
		t.Errorf("GET /health = %d %v, want %v", status, st, want)
	}
	connected.Store(true)
	if _, st := call(t, ts, "GET", "/health", ""); st["arbiter_connected"] != true {
		t.Errorf("arbiter_connected = %v después de conectar", st["arbiter_connected"])
	}
	// El 405 lo genera el router de Go (texto plano); el contrato no define su cuerpo, solo se verifica el status.
	resp, err := http.Post(ts.URL+"/health", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST /health = %d, want 405", resp.StatusCode)
	}
}

// Mientras el bot piensa en una partida, las otras partidas responden sin esperar.
func TestThinkingDoesNotBlockOtherGames(t *testing.T) {
	rec := newRecorder()
	_, ts := newTestAPI(t, rec.choose)
	other := create(t, ts, `{"first":"human"}`)["id"].(string)
	slow := create(t, ts, `{"first":"human"}`)["id"].(string)

	release := make(chan struct{})
	rec.mu.Lock()
	rec.blockOnce = release
	rec.mu.Unlock()

	done := make(chan int, 1)
	go func() { // sin usar t: t.Fatal solo puede llamarse desde la goroutine del test
		resp, err := http.Post(ts.URL+"/local/games/"+slow+"/moves", "application/json", strings.NewReader(`{"column":3}`))
		if err != nil {
			done <- -1
			return
		}
		resp.Body.Close()
		done <- resp.StatusCode
	}()
	time.Sleep(100 * time.Millisecond) // el bot de "slow" queda pensando

	begin := time.Now()
	if status, _ := play(t, ts, other, 2); status != http.StatusOK {
		t.Errorf("la otra partida respondió %d", status)
	}
	if elapsed := time.Since(begin); elapsed > 500*time.Millisecond {
		t.Errorf("la otra partida esperó %v al bot de la partida lenta", elapsed)
	}
	close(release)
	if status := <-done; status != http.StatusOK {
		t.Errorf("la partida lenta respondió %d", status)
	}
}

// Dos jugadas simultáneas en la misma partida se procesan una detrás de la otra.
func TestConcurrentMovesSameGame(t *testing.T) {
	_, ts := newTestAPI(t, newRecorder().choose)
	id := create(t, ts, `{"first":"human"}`)["id"].(string)

	var wg sync.WaitGroup // espera a que terminen las dos goroutines
	statuses := make(chan int, 2)
	for _, col := range []int{3, 4} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Post(ts.URL+"/local/games/"+id+"/moves", "application/json", strings.NewReader(fmt.Sprintf(`{"column":%d}`, col)))
			if err != nil {
				statuses <- -1
				return
			}
			resp.Body.Close()
			statuses <- resp.StatusCode
		}()
	}
	wg.Wait()
	close(statuses)
	for s := range statuses {
		if s != http.StatusOK {
			t.Errorf("status = %d", s)
		}
	}
	_, st := call(t, ts, "GET", "/local/games/"+id, "")
	if h := ints(st["history"]); len(h) != 4 || boardOf(t, st) != rebuild(t, h) {
		t.Errorf("history = %v: dos jugadas del humano y dos respuestas, coherentes con el tablero", h)
	}
}

func TestParallelGames(t *testing.T) {
	_, ts := newTestAPI(t, newRecorder().choose)
	for i := range 8 {
		t.Run(fmt.Sprintf("game-%d", i), func(t *testing.T) {
			t.Parallel()
			first := []string{"human", "bot"}[i%2]
			playUntilEnd(t, ts, create(t, ts, `{"first":"`+first+`"}`))
		})
	}
}
