package main

import (
	"bytes"
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
	"testing"
	"time"

	"github.com/mmontiniyumpii/free-fall/bot/internal/game"
)

// fakeClock es un reloj controlado por el test. Tiene su propio mutex porque lo leen las goroutines del servidor.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newTestServer(t *testing.T) (*server, *httptest.Server, *fakeClock) {
	t.Helper()
	clock := &fakeClock{t: time.Date(2026, 11, 5, 15, 4, 5, 123_000_000, time.UTC)}
	s := newServer(0, 42, clock.Now)
	ts := httptest.NewServer(s.routes())
	t.Cleanup(ts.Close)
	return s, ts, clock
}

// call hace un request y devuelve el status y el cuerpo crudo. token vacío = sin Authorization.
func call(t *testing.T, ts *httptest.Server, method, path, token, body string) (int, []byte) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, ts.URL+path, r)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
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
	return resp.StatusCode, data
}

func asMap(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("respuesta no es un objeto JSON: %v\n%s", err, data)
	}
	return m
}

func fixture(t *testing.T, name string) map[string]any {
	t.Helper()
	data, err := os.ReadFile("../../internal/arbiter/testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return asMap(t, data)
}

// sameShape exige las mismas claves y los mismos tipos JSON que el fixture, recursivamente.
// Las claves en nullable pueden venir en null aunque el fixture traiga un valor.
func sameShape(t *testing.T, where string, got, want any, nullable ...string) {
	t.Helper()
	nullOK := map[string]bool{}
	for _, k := range nullable {
		nullOK[k] = true
	}
	var walk func(path string, got, want any)
	walk = func(path string, got, want any) {
		key := path[strings.LastIndex(path, ".")+1:]
		if got == nil && (want == nil || nullOK[key]) {
			return
		}
		if reflect.TypeOf(got) != reflect.TypeOf(want) {
			t.Errorf("%s%s: tipo %T, el fixture tiene %T", where, path, got, want)
			return
		}
		switch w := want.(type) {
		case map[string]any:
			g := got.(map[string]any)
			for k := range w {
				if _, ok := g[k]; !ok {
					t.Errorf("%s%s: falta la clave %q", where, path, k)
				}
			}
			for k := range g {
				if _, ok := w[k]; !ok {
					t.Errorf("%s%s: clave %q que no está en el fixture", where, path, k)
				}
			}
			for k := range w {
				if _, ok := g[k]; ok {
					walk(path+"."+k, g[k], w[k])
				}
			}
		case []any:
			g := got.([]any)
			if len(g) > 0 && len(w) > 0 {
				walk(path+"[]", g[0], w[0])
			}
		}
	}
	walk("", got, want)
}

type turnResp struct {
	Status   string     `json:"status"`
	GameID   string     `json:"game_id"`
	MatchID  *string    `json:"match_id"`
	Kind     string     `json:"kind"`
	You      int        `json:"you"`
	Turn     int        `json:"turn"`
	Board    game.Board `json:"board"`
	Moves    []int      `json:"moves"`
	LastMove *struct {
		By     int `json:"by"`
		Column int `json:"column"`
	} `json:"last_move"`
	LegalMoves []int  `json:"legal_moves"`
	DeadlineMS int    `json:"deadline_ms"`
	ServedAt   string `json:"served_at"`
}

func next(t *testing.T, ts *httptest.Server, token string) (int, turnResp, []byte) {
	t.Helper()
	status, data := call(t, ts, "GET", "/api/next?wait=0", token, "")
	var tr turnResp
	if status == http.StatusOK {
		if err := json.Unmarshal(data, &tr); err != nil {
			t.Fatal(err)
		}
	}
	return status, tr, data
}

func practice(t *testing.T, ts *httptest.Server, token, opponent string, first int) string {
	t.Helper()
	status, data := call(t, ts, "POST", "/api/practice", token, fmt.Sprintf(`{"opponent":%q,"first":%d}`, opponent, first))
	if status != http.StatusCreated {
		t.Fatalf("practice: status %d: %s", status, data)
	}
	return asMap(t, data)["game_id"].(string)
}

func move(t *testing.T, ts *httptest.Server, token, gameID string, col int) (int, map[string]any) {
	t.Helper()
	status, data := call(t, ts, "POST", "/api/move", token, fmt.Sprintf(`{"game_id":%q,"column":%d}`, gameID, col))
	return status, asMap(t, data)
}

func getGame(t *testing.T, ts *httptest.Server, id string) map[string]any {
	t.Helper()
	status, data := call(t, ts, "GET", "/api/games/"+id, "", "")
	if status != http.StatusOK {
		t.Fatalf("GET game: status %d: %s", status, data)
	}
	return asMap(t, data)
}

// rebuild reconstruye el tablero jugando las columnas en orden, alternando 1 y 2.
func rebuild(t *testing.T, moves []int) game.Board {
	t.Helper()
	var b game.Board
	for i, c := range moves {
		var err error
		b, _, err = game.Apply(b, game.Column(c), game.Player(i%2+1))
		if err != nil {
			t.Fatalf("jugada %d (columna %d): %v", i, c, err)
		}
	}
	return b
}

func TestUnauthorized(t *testing.T) {
	_, ts, _ := newTestServer(t)
	want := fixture(t, "error-unauthorized.json")
	for _, header := range []string{"", "Bearer ", "Bearer    ", "Basic abc", "tok"} {
		req, _ := http.NewRequest("GET", ts.URL+"/api/next?wait=0", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Authorization %q: status %d, want 401", header, resp.StatusCode)
			continue
		}
		if got := asMap(t, data); !reflect.DeepEqual(got, want) {
			t.Errorf("Authorization %q: body %v, want %v", header, got, want)
		}
	}
}

func TestIdleMatchesFixture(t *testing.T) {
	_, ts, _ := newTestServer(t)
	status, _, data := next(t, ts, "tok")
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	if got, want := asMap(t, data), fixture(t, "next-idle.json"); !reflect.DeepEqual(got, want) {
		t.Errorf("idle = %v, want %v", got, want)
	}
}

func TestPracticeTeamOpens(t *testing.T) {
	_, ts, clock := newTestServer(t)
	status, data := call(t, ts, "POST", "/api/practice", "tok", `{"opponent":"casa-basico","first":1}`)
	if status != http.StatusCreated {
		t.Fatalf("status %d: %s", status, data)
	}
	created := asMap(t, data)
	sameShape(t, "practice", created, fixture(t, "practice-created.json"))
	if created["you"] != float64(1) {
		t.Errorf("you = %v, want 1", created["you"])
	}

	status, tr, raw := next(t, ts, "tok")
	if status != http.StatusOK || tr.Status != "turn" {
		t.Fatalf("next: %d %s", status, raw)
	}
	sameShape(t, "turn", asMap(t, raw), fixture(t, "next-turn.json"), "match_id", "last_move")
	m := asMap(t, raw)
	if m["match_id"] != nil || m["last_move"] != nil {
		t.Errorf("match_id=%v last_move=%v, want null y null en el primer turno de una práctica", m["match_id"], m["last_move"])
	}
	if tr.GameID != created["game_id"] || tr.Kind != "practice" || tr.You != 1 || tr.Turn != 1 || len(tr.Moves) != 0 || tr.DeadlineMS != 3000 {
		t.Errorf("turno inesperado: %+v", tr)
	}
	if tr.Board != (game.Board{}) || !reflect.DeepEqual(tr.LegalMoves, []int{0, 1, 2, 3, 4, 5, 6, 7}) {
		t.Errorf("tablero o legal_moves inesperados: %+v", tr)
	}
	if want := clock.Now().Format("2006-01-02T15:04:05.000Z07:00"); tr.ServedAt != want {
		t.Errorf("served_at = %q, want %q", tr.ServedAt, want)
	}
}

func TestPracticeHouseOpens(t *testing.T) {
	_, ts, _ := newTestServer(t)
	practice(t, ts, "tok", "casa-aleatorio", 2)
	_, tr, raw := next(t, ts, "tok")
	if tr.You != 2 || tr.Turn != 2 || len(tr.Moves) != 1 {
		t.Fatalf("turno inesperado: %s", raw)
	}
	if tr.LastMove == nil || tr.LastMove.By != 1 || tr.LastMove.Column != tr.Moves[0] {
		t.Errorf("last_move = %+v, want la apertura de la casa (columna %d)", tr.LastMove, tr.Moves[0])
	}
	if tr.Board != rebuild(t, tr.Moves) {
		t.Errorf("board no coincide con moves")
	}
}

func TestLongPollWaitsThenIdle(t *testing.T) {
	_, ts, _ := newTestServer(t)
	start := time.Now()
	status, data := call(t, ts, "GET", "/api/next?wait=1", "tok", "")
	elapsed := time.Since(start)
	if status != http.StatusOK || asMap(t, data)["status"] != "idle" {
		t.Fatalf("status %d: %s", status, data)
	}
	if elapsed < 900*time.Millisecond {
		t.Errorf("respondió en %v; debía retener la conexión ~1 s", elapsed)
	}
}

func TestLongPollWakesUpOnNewTurn(t *testing.T) {
	_, ts, _ := newTestServer(t)
	type result struct {
		status  int
		data    []byte
		elapsed time.Duration
	}
	done := make(chan result, 1) // canal: la goroutine le pasa el resultado al test
	go func() {
		// Esta goroutine no usa t: t.Fatal solo puede llamarse desde la goroutine del test.
		start := time.Now()
		req, _ := http.NewRequest("GET", ts.URL+"/api/next?wait=10", nil)
		req.Header.Set("Authorization", "Bearer tok")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			done <- result{status: -1, data: []byte(err.Error())}
			return
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		done <- result{resp.StatusCode, data, time.Since(start)}
	}()
	time.Sleep(200 * time.Millisecond) // deja que el long poll quede esperando
	id := practice(t, ts, "tok", "casa-basico", 1)

	r := <-done // bloquea hasta que la goroutine mande el resultado
	if r.status != http.StatusOK {
		t.Fatalf("long poll: %d %s", r.status, r.data)
	}
	if m := asMap(t, r.data); m["status"] != "turn" || m["game_id"] != id {
		t.Fatalf("long poll: %d %s", r.status, r.data)
	}
	if r.elapsed > 3*time.Second {
		t.Errorf("tardó %v; debía responder apenas apareció el turno", r.elapsed)
	}
}

func TestNextValidation(t *testing.T) {
	_, ts, _ := newTestServer(t)
	for _, q := range []string{"wait=abc", "wait=26", "wait=-1"} {
		status, data := call(t, ts, "GET", "/api/next?"+q, "tok", "")
		if status != http.StatusBadRequest || asMap(t, data)["error"] != "validation" {
			t.Errorf("%s: %d %s, want 400 validation", q, status, data)
		}
	}
}

func TestPracticeValidation(t *testing.T) {
	_, ts, _ := newTestServer(t)
	for _, body := range []string{`{"opponent":"casa-fuerte"}`, `{"opponent":""}`, `{"opponent":"casa-basico","first":3}`, `not json`} {
		status, data := call(t, ts, "POST", "/api/practice", "tok", body)
		if status != http.StatusBadRequest || asMap(t, data)["error"] != "validation" {
			t.Errorf("body %s: %d %s, want 400 validation", body, status, data)
		}
	}
	// first es opcional: por defecto abre el equipo.
	status, data := call(t, ts, "POST", "/api/practice", "tok", `{"opponent":"casa-basico"}`)
	if status != http.StatusCreated || asMap(t, data)["you"] != float64(1) {
		t.Errorf("sin first: %d %s, want 201 you=1", status, data)
	}
	// opponent también es opcional: por defecto, casa-basico.
	status, data = call(t, ts, "POST", "/api/practice", "tok", `{}`)
	if status != http.StatusCreated {
		t.Fatalf("sin opponent: %d %s, want 201", status, data)
	}
	g := getGame(t, ts, asMap(t, data)["game_id"].(string))
	if rival := g["players"].(map[string]any)["2"].(map[string]any)["team_id"]; rival != "casa-basico" {
		t.Errorf("sin opponent, el rival es %v; want casa-basico", rival)
	}
}

// playGame juega una partida completa eligiendo siempre la primera columna legal,
// y en cada turno verifica que el mock entregue un estado coherente.
func playGame(t *testing.T, ts *httptest.Server, token, opponent string, first int) string {
	t.Helper()
	id := practice(t, ts, token, opponent, first)
	for i := 0; ; i++ {
		if i > game.Cells {
			t.Fatal("la partida no termina")
		}
		status, tr, raw := next(t, ts, token)
		if status != http.StatusOK {
			t.Fatalf("next: %d %s", status, raw)
		}
		if tr.Status == "idle" {
			break
		}
		if tr.Board != rebuild(t, tr.Moves) {
			t.Fatalf("board no coincide con moves: %s", raw)
		}
		if tr.Turn != len(tr.Moves)+1 || tr.You != len(tr.Moves)%2+1 {
			t.Fatalf("turn/you incoherentes con moves: %s", raw)
		}
		legal := make([]int, 0, game.Cols)
		for _, c := range game.LegalMoves(tr.Board) {
			legal = append(legal, int(c))
		}
		if !reflect.DeepEqual(tr.LegalMoves, legal) {
			t.Fatalf("legal_moves = %v, want %v", tr.LegalMoves, legal)
		}
		if st, body := move(t, ts, token, tr.GameID, tr.LegalMoves[0]); st != http.StatusOK {
			t.Fatalf("move: %d %v", st, body)
		}
	}
	return id
}

func TestFullGames(t *testing.T) {
	_, ts, _ := newTestServer(t)
	for _, opponent := range []string{"casa-aleatorio", "casa-basico"} {
		for _, first := range []int{1, 2} {
			t.Run(fmt.Sprintf("%s/first=%d", opponent, first), func(t *testing.T) {
				id := playGame(t, ts, "tok", opponent, first)
				g := getGame(t, ts, id)
				if g["status"] != "finished" || g["finished_at"] == nil {
					t.Fatalf("partida no terminada: %v", g)
				}
				var moves []int
				for _, m := range g["moves"].([]any) {
					moves = append(moves, int(m.(map[string]any)["column"].(float64)))
				}
				var board game.Board
				data, _ := json.Marshal(g["board"])
				if err := json.Unmarshal(data, &board); err != nil {
					t.Fatal(err)
				}
				if board != rebuild(t, moves) {
					t.Errorf("board final no coincide con moves")
				}
				result := g["result"].(map[string]any)
				switch result["reason"] {
				case "connect4":
					cells := result["winning_cells"].([]any)
					winner := game.Cell(result["winner"].(float64))
					if len(cells) != 4 {
						t.Fatalf("winning_cells = %v", cells)
					}
					for _, c := range cells {
						rc := c.([]any)
						if board[int(rc[0].(float64))][int(rc[1].(float64))] != winner {
							t.Errorf("celda %v no es del ganador %d", rc, winner)
						}
					}
				case "draw":
					if result["winner"] != nil || len(moves) != game.Cells {
						t.Errorf("empate incoherente: %v", result)
					}
				default:
					t.Errorf("reason = %v, want connect4 o draw", result["reason"])
				}
			})
		}
	}
}

// inject reemplaza el estado de una partida para armar escenarios concretos.
func inject(t *testing.T, s *server, id string, b game.Board, toMove game.Player) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.games[id]
	g.board = b
	g.moves = g.moves[:0]
	for r := range game.Rows {
		for c := range game.Cols {
			if b[r][c] != game.Empty {
				g.moves = append(g.moves, moveRecord{Turn: len(g.moves) + 1, By: game.Player(b[r][c]), Column: c})
			}
		}
	}
	g.toMove = toMove
	g.availableAt = s.now()
	g.servedAt = time.Time{}
}

func TestMovePlaying(t *testing.T) {
	_, ts, _ := newTestServer(t)
	id := practice(t, ts, "tok", "casa-basico", 1)
	next(t, ts, "tok")
	status, body := move(t, ts, "tok", id, 3)
	if status != http.StatusOK {
		t.Fatalf("status %d: %v", status, body)
	}
	sameShape(t, "move", body, fixture(t, "move-playing.json"))
	if body["state"] != "playing" || body["turn"] != float64(2) {
		t.Errorf("body = %v, want playing turn 2", body)
	}
}

func TestMoveWon(t *testing.T) {
	s, ts, _ := newTestServer(t)
	id := practice(t, ts, "tok", "casa-basico", 1)
	inject(t, s, id, board(t, e, e, e, e, e, "222.....", "111....."), 1)
	next(t, ts, "tok")

	status, body := move(t, ts, "tok", id, 3)
	if status != http.StatusOK {
		t.Fatalf("status %d: %v", status, body)
	}
	sameShape(t, "move", body, fixture(t, "move-won.json"))
	wantCells := []any{[]any{0.0, 0.0}, []any{0.0, 1.0}, []any{0.0, 2.0}, []any{0.0, 3.0}}
	if body["state"] != "won" || body["turn"] != float64(7) || !reflect.DeepEqual(body["winning_cells"], wantCells) {
		t.Errorf("body = %v", body)
	}

	g := getGame(t, ts, id)
	// house solo aparece en el bot de la casa (schema PlayerRef): se verifica aparte del fixture, que no lo trae.
	players := g["players"].(map[string]any)
	if house := players["2"].(map[string]any); house["house"] != true || house["team_id"] != "casa-basico" {
		t.Errorf("jugador de la casa = %v, want team_id casa-basico y house true", house)
	}
	if _, ok := players["1"].(map[string]any)["house"]; ok {
		t.Errorf("el equipo no debe traer house: %v", players["1"])
	}
	delete(players["2"].(map[string]any), "house")
	sameShape(t, "game", g, fixture(t, "game.json"), "match_id")
	if g["current"] != nil || g["series"] != nil {
		t.Errorf("partida terminada: current = %v, series = %v; want null", g["current"], g["series"])
	}
	if r := g["result"].(map[string]any); r["winner"] != float64(1) || r["reason"] != "connect4" {
		t.Errorf("result = %v", r)
	}
	if st, _, raw := next(t, ts, "tok"); st != http.StatusOK || asMap(t, raw)["status"] != "idle" {
		t.Errorf("después de ganar, next = %s; want idle", raw)
	}
}

func TestMoveDraw(t *testing.T) {
	s, ts, _ := newTestServer(t)
	id := practice(t, ts, "tok", "casa-basico", 2) // el equipo es el jugador 2
	inject(t, s, id, board(t,
		"2112212.", // falta la última casilla, que es del 2
		"11212211",
		"21221112",
		"12111222",
		"12221121",
		"12112111",
		"21222122",
	), 2)
	next(t, ts, "tok")

	status, body := move(t, ts, "tok", id, 7)
	if status != http.StatusOK {
		t.Fatalf("status %d: %v", status, body)
	}
	sameShape(t, "move", body, fixture(t, "move-draw.json"))
	if body["state"] != "draw" || body["turn"] != float64(56) {
		t.Errorf("body = %v, want draw turn 56", body)
	}
	if r := getGame(t, ts, id)["result"].(map[string]any); r["winner"] != nil || r["reason"] != "draw" {
		t.Errorf("result = %v", r)
	}
}

func TestMoveIllegalLosesTheGame(t *testing.T) {
	s, ts, _ := newTestServer(t)
	fullCol0 := board(t, "1.......", "2.......", "1.......", "2.......", "1.......", "2.......", "1.......")

	tests := []struct {
		name  string
		setup func(id string)
		col   int
	}{
		{"column out of range", func(string) {}, 9},
		{"negative column", func(string) {}, -1},
		{"full column", func(id string) { inject(t, s, id, fullCol0, 2) }, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			token := "tok-" + strings.ReplaceAll(tc.name, " ", "-")
			id := practice(t, ts, token, "casa-basico", 2)
			tc.setup(id)
			next(t, ts, token)
			status, body := move(t, ts, token, id, tc.col)
			if status != http.StatusBadRequest {
				t.Fatalf("status %d: %v", status, body)
			}
			sameShape(t, "error", body, fixture(t, "error-illegal-move.json"))
			if body["error"] != "illegal_move" {
				t.Errorf("error = %v", body["error"])
			}
			r := getGame(t, ts, id)["result"].(map[string]any)
			if r["reason"] != "illegal_move" || r["winner"] != float64(1) || r["forfeited_by"] != float64(2) ||
				r["column"] != float64(tc.col) || r["ms"] == nil {
				t.Errorf("result = %v, want derrota por illegal_move con forfeited_by 2, column %d y ms", r, tc.col)
			}
		})
	}
}

func TestMoveValidationDoesNotLose(t *testing.T) {
	_, ts, _ := newTestServer(t)
	id := practice(t, ts, "tok", "casa-basico", 1)
	next(t, ts, "tok")
	for _, body := range []string{`not json`, `{"game_id":"` + id + `"}`, `{"column":3}`, `{"game_id":"` + id + `","column":"3"}`} {
		status, data := call(t, ts, "POST", "/api/move", "tok", body)
		if status != http.StatusBadRequest || asMap(t, data)["error"] != "validation" {
			t.Errorf("body %s: %d %s, want 400 validation", body, status, data)
		}
	}
	if g := getGame(t, ts, id); g["status"] != "playing" {
		t.Errorf("un body inválido no debe terminar la partida: %v", g["result"])
	}
}

func TestMoveNotYourTurn(t *testing.T) {
	_, ts, _ := newTestServer(t)
	status, body := move(t, ts, "tok", "g_no_existe", 0)
	if status != http.StatusConflict || body["error"] != "unknown_game" {
		t.Fatalf("partida inexistente: %d %v, want 409 unknown_game", status, body)
	}
	sameShape(t, "error", body, fixture(t, "error-unknown-game.json"))

	// Partida de otro equipo.
	id := practice(t, ts, "tok-a", "casa-basico", 1)
	status, body = move(t, ts, "tok-b", id, 0)
	if status != http.StatusConflict || body["error"] != "not_your_turn" {
		t.Errorf("partida ajena: %d %v", status, body)
	}
	sameShape(t, "error", body, fixture(t, "error-not-your-turn.json"))

	// Partida terminada.
	move(t, ts, "tok-a", id, 9) // jugada ilegal: pierde
	if status, body := move(t, ts, "tok-a", id, 0); status != http.StatusConflict || body["error"] != "not_your_turn" {
		t.Errorf("partida terminada: %d %v", status, body)
	}
}

func TestMoveTimeout(t *testing.T) {
	_, ts, clock := newTestServer(t)

	// Justo en el límite se acepta.
	id := practice(t, ts, "tok-a", "casa-basico", 1)
	next(t, ts, "tok-a")
	clock.Advance(3000 * time.Millisecond)
	if status, body := move(t, ts, "tok-a", id, 3); status != http.StatusOK {
		t.Fatalf("a 3000 ms: %d %v, want 200", status, body)
	}

	// Un milisegundo después del límite se pierde.
	id = practice(t, ts, "tok-b", "casa-basico", 1)
	next(t, ts, "tok-b")
	clock.Advance(3001 * time.Millisecond)
	status, body := move(t, ts, "tok-b", id, 3)
	if status != http.StatusBadRequest || body["error"] != "timeout" {
		t.Fatalf("a 3001 ms: %d %v, want 400 timeout", status, body)
	}
	sameShape(t, "error", body, fixture(t, "error-timeout.json"))
	if r := getGame(t, ts, id)["result"].(map[string]any); r["reason"] != "timeout" || r["winner"] != float64(2) ||
		r["forfeited_by"] != float64(1) || r["ms"] != float64(3001) {
		t.Errorf("result = %v, want timeout con forfeited_by 1 y ms 3001", r)
	}
}

func TestServedAtDoesNotChangeOnRepeatedNext(t *testing.T) {
	_, ts, clock := newTestServer(t)
	practice(t, ts, "tok", "casa-basico", 1)
	_, first, _ := next(t, ts, "tok")
	clock.Advance(time.Second)
	_, second, _ := next(t, ts, "tok")
	if first.ServedAt != second.ServedAt {
		t.Errorf("served_at cambió de %s a %s; el reloj corre desde la primera entrega", first.ServedAt, second.ServedAt)
	}
}

func TestAbsentLosesTheGame(t *testing.T) {
	_, ts, clock := newTestServer(t)
	id := practice(t, ts, "tok", "casa-basico", 1)
	clock.Advance(29 * time.Second)
	if _, tr, _ := next(t, ts, "tok"); tr.Status != "turn" {
		t.Fatal("a los 29 s el turno debe seguir disponible")
	}

	id2 := practice(t, ts, "tok-2", "casa-basico", 1)
	clock.Advance(31 * time.Second)
	if _, tr, _ := next(t, ts, "tok-2"); tr.Status != "idle" {
		t.Fatalf("a los 31 s sin recoger el turno, next = %+v; want idle", tr)
	}
	if r := getGame(t, ts, id2)["result"].(map[string]any); r["reason"] != "absent" || r["winner"] != float64(2) ||
		r["forfeited_by"] != float64(1) || r["waited_ms"] != float64(31000) {
		t.Errorf("result = %v, want derrota por absent con forfeited_by 1 y waited_ms 31000", r)
	}
	if g := getGame(t, ts, id); g["status"] != "playing" {
		t.Errorf("un turno ya entregado no vence por ausencia: %v", g["result"])
	}
}

func TestOldestTurnFirst(t *testing.T) {
	_, ts, clock := newTestServer(t)
	older := practice(t, ts, "tok", "casa-basico", 1)
	clock.Advance(time.Second)
	practice(t, ts, "tok", "casa-basico", 1)
	if _, tr, _ := next(t, ts, "tok"); tr.GameID != older {
		t.Errorf("next devolvió %s, want la partida más antigua %s", tr.GameID, older)
	}
}

func TestMeAndSubmission(t *testing.T) {
	_, ts, _ := newTestServer(t)
	id := practice(t, ts, "tok", "casa-basico", 1)

	status, data := call(t, ts, "GET", "/api/me", "tok", "")
	if status != http.StatusOK {
		t.Fatalf("me: %d %s", status, data)
	}
	me := asMap(t, data)
	sameShape(t, "me", me, fixture(t, "me.json"), "submission")
	if me["phase"] != "practice" {
		t.Errorf("phase = %v, want practice", me["phase"])
	}
	if !slices.Contains(me["pending_turns"].([]any), any(id)) {
		t.Errorf("pending_turns = %v, want que incluya %s", me["pending_turns"], id)
	}

	for _, body := range []string{
		`{"repo_url":"","commit":"a1b2c3d"}`,
		`{"repo_url":"https://github.com/x/y","commit":"abc"}`,
		`{"repo_url":"https://github.com/x/y","commit":"a1b2c3z"}`, // no es hexadecimal
		`{"repo_url":"no-es-url","commit":"a1b2c3d"}`,
		`{"repo_url":"https://gitlab.com/x/y","commit":"a1b2c3d"}`, // solo github.com o bitbucket.org
	} {
		if status, data := call(t, ts, "PUT", "/api/me/submission", "tok", body); status != http.StatusBadRequest {
			t.Errorf("submission %s: %d %s, want 400", body, status, data)
		}
	}
	status, data = call(t, ts, "PUT", "/api/me/submission", "tok", `{"repo_url":"https://github.com/x/y","commit":"a1b2c3d4e5f6"}`)
	if status != http.StatusOK {
		t.Fatalf("submission: %d %s", status, data)
	}
	sub := asMap(t, data)
	if sub["commit"] != "a1b2c3d4e5f6" || sub["registered_at"] == nil {
		t.Errorf("submission = %v", sub)
	}
	_, data = call(t, ts, "GET", "/api/me", "tok", "")
	sameShape(t, "me", asMap(t, data), fixture(t, "me.json"))
}

func TestPublicEndpoints(t *testing.T) {
	_, ts, _ := newTestServer(t)
	if status, _ := call(t, ts, "GET", "/api/games/g_no_existe", "", ""); status != http.StatusNotFound {
		t.Errorf("game inexistente: %d, want 404", status)
	}
	status, data := call(t, ts, "GET", "/api/standings", "", "")
	if status != http.StatusOK {
		t.Fatalf("standings: %d", status)
	}
	if m := asMap(t, data); m["phase"] != "practice" || m["standings"] == nil {
		t.Errorf("standings = %v", m)
	}
}

func TestGameWhilePlaying(t *testing.T) {
	_, ts, _ := newTestServer(t)
	id := practice(t, ts, "tok", "casa-basico", 2) // abre la casa: después le toca al equipo, que es el 2
	g := getGame(t, ts, id)
	if g["status"] != "playing" || g["current"] != float64(2) || g["series"] != nil || g["result"] != nil {
		t.Errorf("partida en curso = %v, want status playing, current 2, series y result null", g)
	}
}

func TestHealth(t *testing.T) {
	_, ts, _ := newTestServer(t)
	status, data := call(t, ts, "GET", "/api/health", "", "")
	if status != http.StatusOK {
		t.Fatalf("health: %d %s", status, data)
	}
	h := asMap(t, data)
	sameShape(t, "health", h, fixture(t, "health.json"))
	if h["ok"] != true || h["phase"] != "practice" || h["time"] != "2026-11-05T15:04:05.123Z" {
		t.Errorf("health = %v", h)
	}
}

func listGames(t *testing.T, ts *httptest.Server, query string) []map[string]any {
	t.Helper()
	status, data := call(t, ts, "GET", "/api/games"+query, "", "")
	if status != http.StatusOK {
		t.Fatalf("GET /api/games%s: %d %s", query, status, data)
	}
	var list []map[string]any
	if err := json.Unmarshal(data, &list); err != nil {
		t.Fatalf("la lista no es un array JSON: %v\n%s", err, data)
	}
	return list
}

func ids(list []map[string]any) []string {
	out := make([]string, len(list))
	for i, g := range list {
		out[i], _ = g["game_id"].(string)
	}
	return out
}

func TestListGames(t *testing.T) {
	_, ts, _ := newTestServer(t)
	if list := listGames(t, ts, ""); len(list) != 0 {
		t.Fatalf("sin partidas, la lista = %v; want []", list)
	}
	a, b := teamID(t, ts, "tok-a"), teamID(t, ts, "tok-b")
	p1 := practice(t, ts, "tok-a", "casa-basico", 1)
	p2 := practice(t, ts, "tok-b", "casa-aleatorio", 2)
	_, data := call(t, ts, "POST", "/mock/friendly", "", fmt.Sprintf(`{"player1":%q,"player2":%q}`, a, b))
	f := asMap(t, data)["game_id"].(string)

	list := listGames(t, ts, "")
	if got, want := ids(list), []string{f, p2, p1}; !slices.Equal(got, want) {
		t.Errorf("orden = %v, want %v (más recientes primero)", got, want)
	}
	// GameSummary es Game sin board (allOf en el OpenAPI): mismas claves que el fixture salvo board.
	summary := fixture(t, "game.json")
	delete(summary, "board")
	// El ejemplo es de una partida terminada (current null); estas están en curso, así que current es un entero.
	summary["current"] = float64(1)
	for _, g := range list {
		if _, ok := g["board"]; ok {
			t.Errorf("%s: la lista no debe traer board", g["game_id"])
		}
		for _, p := range g["players"].(map[string]any) {
			delete(p.(map[string]any), "house")
		}
		sameShape(t, "summary", g, summary, "match_id", "result", "current", "finished_at")
	}

	if got := ids(listGames(t, ts, "?kind=practice")); !slices.Equal(got, []string{p2, p1}) {
		t.Errorf("kind=practice: %v, want [%s %s]", got, p2, p1)
	}
	if got := ids(listGames(t, ts, "?kind=friendly&limit=5")); !slices.Equal(got, []string{f}) {
		t.Errorf("kind=friendly: %v, want [%s]", got, f)
	}
	if got := ids(listGames(t, ts, "?limit=1")); !slices.Equal(got, []string{f}) {
		t.Errorf("limit=1: %v, want [%s]", got, f)
	}
	if got := listGames(t, ts, "?kind=league"); len(got) != 0 {
		t.Errorf("kind=league: %v, want []", got)
	}

	for _, q := range []string{"?kind=torneo", "?limit=0", "?limit=501", "?limit=abc"} {
		status, data := call(t, ts, "GET", "/api/games"+q, "", "")
		if status != http.StatusBadRequest || asMap(t, data)["error"] != "validation" {
			t.Errorf("%s: %d %s, want 400 validation", q, status, data)
		}
	}
}

// Varios equipos juegan a la vez: con -race detecta accesos sin mutex al estado del mock.
func TestConcurrentTeams(t *testing.T) {
	_, ts, _ := newTestServer(t)
	for i := range 8 {
		t.Run(fmt.Sprintf("team-%d", i), func(t *testing.T) {
			t.Parallel() // los 8 subtests corren al mismo tiempo, cada uno en su goroutine
			playGame(t, ts, fmt.Sprintf("tok-%d", i), "casa-aleatorio", i%2+1)
		})
	}
}

func teamID(t *testing.T, ts *httptest.Server, token string) string {
	t.Helper()
	status, data := call(t, ts, "GET", "/api/me", token, "")
	if status != http.StatusOK {
		t.Fatalf("me: %d %s", status, data)
	}
	return asMap(t, data)["team"].(map[string]any)["id"].(string)
}

// Una partida entre dos equipos (extensión del mock, fuera del OpenAPI): los dos juegan por /api/next y /api/move.
func TestFriendlyBetweenTwoTeams(t *testing.T) {
	_, ts, _ := newTestServer(t)
	a, b := teamID(t, ts, "tok-a"), teamID(t, ts, "tok-b")

	status, data := call(t, ts, "POST", "/mock/friendly", "", fmt.Sprintf(`{"player1":%q,"player2":%q,"opening":[3,4]}`, a, b))
	if status != http.StatusCreated {
		t.Fatalf("friendly: %d %s", status, data)
	}
	id := asMap(t, data)["game_id"].(string)

	// Después de la apertura [3, 4] le toca al jugador 1; el 2 no tiene turno todavía.
	if _, tr, raw := next(t, ts, "tok-b"); tr.Status != "idle" {
		t.Fatalf("el jugador 2 no debería tener turno: %s", raw)
	}
	if _, tr, raw := next(t, ts, "tok-a"); tr.Status != "turn" || tr.You != 1 || !reflect.DeepEqual(tr.Moves, []int{3, 4}) {
		t.Fatalf("la apertura [3,4] no se aplicó: %s", raw)
	}

	tokens := map[int]string{1: "tok-a", 2: "tok-b"}
	for i := 0; ; i++ {
		if i > game.Cells {
			t.Fatal("la partida no termina")
		}
		moved := false
		for side := 1; side <= 2; side++ {
			_, tr, raw := next(t, ts, tokens[side])
			if tr.Status != "turn" {
				continue
			}
			if tr.GameID != id || tr.You != side || tr.Kind != "friendly" || tr.MatchID == nil || *tr.MatchID != "amistoso" ||
				tr.Board != rebuild(t, tr.Moves) {
				t.Fatalf("turno incoherente para el jugador %d: %s", side, raw)
			}
			if st, body := move(t, ts, tokens[side], id, tr.LegalMoves[0]); st != http.StatusOK {
				t.Fatalf("move: %d %v", st, body)
			}
			moved = true
		}
		if !moved {
			break
		}
	}
	g := getGame(t, ts, id)
	players := g["players"].(map[string]any)
	if g["status"] != "finished" || g["kind"] != "friendly" || g["match_id"] != "amistoso" ||
		players["1"].(map[string]any)["team_id"] != a || players["2"].(map[string]any)["team_id"] != b {
		t.Errorf("partida = %v", g)
	}
}

func TestFriendlyValidation(t *testing.T) {
	_, ts, _ := newTestServer(t)
	a, b := teamID(t, ts, "tok-a"), teamID(t, ts, "tok-b")
	for _, body := range []string{
		`not json`,
		fmt.Sprintf(`{"player1":%q}`, a),
		fmt.Sprintf(`{"player1":%q,"player2":"eq99"}`, a),
		fmt.Sprintf(`{"player1":%q,"player2":%q}`, a, a),
		fmt.Sprintf(`{"player1":%q,"player2":%q,"opening":[9]}`, a, b),
		fmt.Sprintf(`{"player1":%q,"player2":%q,"opening":[0,1,0,1,0,1,0]}`, a, b), // la apertura gana
	} {
		if status, data := call(t, ts, "POST", "/mock/friendly", "", body); status != http.StatusBadRequest {
			t.Errorf("body %s: %d %s, want 400", body, status, data)
		}
	}
}

func TestLatency(t *testing.T) {
	s := newServer(150*time.Millisecond, 1, time.Now)
	ts := httptest.NewServer(s.routes())
	defer ts.Close()
	start := time.Now()
	req, _ := http.NewRequest("POST", ts.URL+"/api/practice", bytes.NewBufferString(`{"opponent":"casa-basico"}`))
	req.Header.Set("Authorization", "Bearer tok")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if elapsed := time.Since(start); elapsed < 150*time.Millisecond {
		t.Errorf("respondió en %v con MOCK_LATENCY_MS=150", elapsed)
	}
}
