package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mmontiniyumpii/free-fall/bot/internal/game"
)

// server guarda todo el estado del mock en memoria.
// mu protege teams, games, created y rng: cada handler corre en su propia goroutine,
// así que todo acceso a ese estado se hace con el mutex tomado (Lock ... Unlock).
type server struct {
	mu      sync.Mutex
	teams   map[string]*team // clave: el token del equipo. Nunca se loguea.
	games   map[string]*mockGame
	created int
	rng     *rand.Rand

	latency time.Duration    // MOCK_LATENCY_MS: demora artificial antes de cada respuesta
	now     func() time.Time // reloj inyectable para los tests
}

func newServer(latency time.Duration, seed uint64, now func() time.Time) *server {
	return &server{
		teams:   map[string]*team{},
		games:   map[string]*mockGame{},
		rng:     rand.New(rand.NewPCG(seed, seed)),
		latency: latency,
		now:     now,
	}
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/next", s.auth(s.handleNext))
	mux.HandleFunc("POST /api/move", s.auth(s.handleMove))
	mux.HandleFunc("POST /api/practice", s.auth(s.handlePractice))
	mux.HandleFunc("GET /api/me", s.auth(s.handleMe))
	mux.HandleFunc("PUT /api/me/submission", s.auth(s.handleSubmission))
	// Públicos: sin token.
	mux.HandleFunc("GET /api/games/{game_id}", s.handleGame)
	mux.HandleFunc("GET /api/standings", s.handleStandings)
	// Extensión del mock, NO es parte del OpenAPI: partidas entre dos equipos para cmd/selfplay.
	mux.HandleFunc("POST /mock/friendly", s.handleFriendly)
	return mux
}

// auth exige "Authorization: Bearer <token>" y le pasa al handler el equipo de ese token.
func (s *server) auth(h func(http.ResponseWriter, *http.Request, *team)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		token = strings.TrimSpace(token)
		if !ok || token == "" {
			s.writeError(w, http.StatusUnauthorized, "unauthorized", "Token inválido.")
			return
		}
		h(w, r, s.teamFor(token))
	}
}

func (s *server) teamFor(token string) *team {
	s.mu.Lock()
	defer s.mu.Unlock() // defer: el Unlock corre al salir de la función, por cualquier return
	t := s.teams[token]
	if t == nil {
		n := len(s.teams) + 1
		t = &team{id: fmt.Sprintf("eq%d", n), name: fmt.Sprintf("Equipo %d", n), notify: make(chan struct{}, 1)}
		s.teams[token] = t
	}
	return t
}

// ---- GET /api/next: long polling ----

type turnResponse struct {
	Status     string      `json:"status"`
	GameID     string      `json:"game_id"`
	MatchID    *string     `json:"match_id"` // puntero: null en prácticas
	Kind       string      `json:"kind"`
	You        game.Player `json:"you"`
	Opponent   string      `json:"opponent"`
	Turn       int         `json:"turn"`
	Board      game.Board  `json:"board"`
	Moves      []int       `json:"moves"` // []int y no []game.Column: un slice de uint8 se serializa como base64
	LastMove   *lastMove   `json:"last_move"`
	LegalMoves []int       `json:"legal_moves"`
	DeadlineMS int         `json:"deadline_ms"`
	ServedAt   string      `json:"served_at"`
}

type lastMove struct {
	By     game.Player `json:"by"`
	Column int         `json:"column"`
}

func (s *server) handleNext(w http.ResponseWriter, r *http.Request, t *team) {
	wait := 20 * time.Second
	if q := r.URL.Query().Get("wait"); q != "" {
		n, err := strconv.Atoi(q)
		if err != nil || n < 0 || n > 25 {
			s.writeError(w, http.StatusBadRequest, "validation", "wait debe ser un entero entre 0 y 25.")
			return
		}
		wait = time.Duration(n) * time.Second
	}

	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		if turn, ok := s.takeTurn(t); ok {
			s.writeJSON(w, http.StatusOK, turn)
			return
		}
		// select espera a que pase la primera de estas tres cosas, sin consumir CPU mientras tanto.
		select {
		case <-t.notify: // apareció un turno para el equipo: vuelve a buscarlo
		case <-timer.C: // se cumplió wait sin novedades
			s.writeJSON(w, http.StatusOK, map[string]string{"status": "idle"})
			return
		case <-r.Context().Done(): // el cliente cortó la conexión
			return
		}
	}
}

// takeTurn busca el turno pendiente más antiguo del equipo y lo marca como entregado.
func (s *server) takeTurn(t *team) (turnResponse, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	var oldest *mockGame
	for _, g := range s.games {
		g.expireAbsent(now)
		if !g.pendingFor(t) {
			continue
		}
		if oldest == nil || g.availableAt.Before(oldest.availableAt) ||
			(g.availableAt.Equal(oldest.availableAt) && g.id < oldest.id) {
			oldest = g
		}
	}
	if oldest == nil {
		return turnResponse{}, false
	}
	if oldest.servedAt.IsZero() { // el reloj corre desde la primera entrega, aunque se pida dos veces
		oldest.servedAt = now
	}

	g := oldest
	moves := make([]int, len(g.moves))
	for i, m := range g.moves {
		moves[i] = m.Column
	}
	var last *lastMove
	if n := len(g.moves); n > 0 {
		last = &lastMove{By: g.moves[n-1].By, Column: g.moves[n-1].Column}
	}
	return turnResponse{
		Status:     "turn",
		GameID:     g.id,
		Kind:       g.kind,
		You:        g.toMove,
		Opponent:   s.nameOf(g, 3-g.toMove),
		Turn:       len(g.moves) + 1,
		Board:      g.board,
		Moves:      moves,
		LastMove:   last,
		LegalMoves: columns(game.LegalMoves(g.board)),
		DeadlineMS: int(deadline / time.Millisecond),
		ServedAt:   formatTime(g.servedAt),
	}, true
}

// ---- POST /api/move ----

type moveAccepted struct {
	OK           bool     `json:"ok"`
	State        string   `json:"state"`
	Turn         int      `json:"turn"`
	WinningCells [][2]int `json:"winning_cells,omitempty"`
}

func (s *server) handleMove(w http.ResponseWriter, r *http.Request, t *team) {
	// Punteros para distinguir "campo ausente" de "valor cero" (column 0 es válida).
	var req struct {
		GameID *string `json:"game_id"`
		Column *int    `json:"column"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil || req.GameID == nil || req.Column == nil {
		s.writeError(w, http.StatusBadRequest, "validation", `El cuerpo debe ser {"game_id": string, "column": entero}.`)
		return
	}

	s.mu.Lock()
	now := s.now()
	g := s.games[*req.GameID]
	if g != nil {
		g.expireAbsent(now)
	}
	if g == nil || !g.pendingFor(t) {
		s.mu.Unlock()
		s.writeError(w, http.StatusConflict, "not_your_turn",
			fmt.Sprintf("La partida %s no tiene un turno pendiente para tu equipo.", *req.GameID))
		return
	}

	side := g.toMove
	served := g.servedAt
	if served.IsZero() { // jugó sin pedir el turno por /api/next: el reloj arranca ahora
		served = now
	}
	elapsed := now.Sub(served)
	col := *req.Column

	var errCode, errMsg string
	switch {
	case elapsed > deadline:
		errCode, errMsg = "timeout", fmt.Sprintf("La jugada llegó %d ms después del límite.", (elapsed-deadline).Milliseconds())
	case col < 0 || col >= game.Cols:
		errCode, errMsg = "illegal_move", fmt.Sprintf("La columna %d está fuera de rango.", col)
	}
	var win []game.Pos
	if errCode == "" {
		var err error
		win, err = g.play(now, side, game.Column(col), elapsed.Milliseconds())
		switch {
		case errors.Is(err, game.ErrColumnFull):
			errCode, errMsg = "illegal_move", fmt.Sprintf("La columna %d está llena.", col)
		case err != nil:
			errCode, errMsg = "illegal_move", err.Error()
		}
	}
	if errCode != "" {
		g.finish(now, 3-side, errCode, nil) // jugada rechazada: la partida se pierde
		s.mu.Unlock()
		slog.Info("game finished", "game_id", g.id, "reason", errCode)
		s.writeError(w, http.StatusBadRequest, errCode, errMsg)
		return
	}

	// El estado es el de la partida para el equipo justo después de su jugada, antes de que responda la casa.
	resp := moveAccepted{OK: true, State: "playing", Turn: len(g.moves) + 1}
	if !g.playing() {
		resp.Turn = len(g.moves)
		resp.State = "draw"
		if win != nil {
			resp.State = "won"
			resp.WinningCells = g.result.WinningCells
		}
	}
	s.advance(g, now)
	s.mu.Unlock()
	s.writeJSON(w, http.StatusOK, resp)
}

// advance hace jugar a la casa mientras le toque y avisa al equipo cuando tiene turno. Requiere s.mu tomado.
func (s *server) advance(g *mockGame, now time.Time) {
	for g.playing() {
		next := g.players[g.toMove]
		if next != nil {
			// Envío sin bloquear: si ya había un aviso pendiente en el canal, no hace falta otro.
			select {
			case next.notify <- struct{}{}:
			default:
			}
			return
		}
		side := g.toMove
		if _, err := g.play(now, side, houseMove(g.house, g.board, side, s.rng), 0); err != nil {
			slog.Error("house move", "game_id", g.id, "err", err)
			g.finish(now, 3-side, "illegal_move", nil)
		}
	}
}

// ---- POST /api/practice ----

func (s *server) handlePractice(w http.ResponseWriter, r *http.Request, t *team) {
	var req struct {
		Opponent string `json:"opponent"`
		First    *int   `json:"first"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "validation", "El cuerpo debe ser JSON.")
		return
	}
	if req.Opponent != houseRandom && req.Opponent != houseBasic {
		s.writeError(w, http.StatusBadRequest, "validation", "opponent debe ser casa-aleatorio o casa-basico.")
		return
	}
	side := game.Player(1) // first: 1 abre el equipo, 2 abre la casa; por defecto 1
	if req.First != nil {
		if *req.First != 1 && *req.First != 2 {
			s.writeError(w, http.StatusBadRequest, "validation", "first debe ser 1 o 2.")
			return
		}
		side = game.Player(*req.First)
	}

	s.mu.Lock()
	now := s.now()
	s.created++
	g := &mockGame{
		id:          fmt.Sprintf("g_p_%02d", s.created),
		kind:        "practice",
		house:       req.Opponent,
		toMove:      1,
		availableAt: now,
		startedAt:   now,
	}
	g.players[side] = t
	s.games[g.id] = g
	s.advance(g, now) // si abre la casa, juega ahora; si abre el equipo, le avisa
	s.mu.Unlock()

	s.writeJSON(w, http.StatusCreated, struct {
		GameID string      `json:"game_id"`
		You    game.Player `json:"you"`
	}{g.id, side})
}

// ---- POST /mock/friendly (extensión del mock) ----

// handleFriendly crea una partida entre dos equipos ya registrados (por su id de GET /api/me).
// opening son columnas que se juegan antes de entregar el primer turno, para que dos motores
// deterministas no repitan siempre la misma partida.
func (s *server) handleFriendly(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Player1 string `json:"player1"`
		Player2 string `json:"player2"`
		Opening []int  `json:"opening"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "validation", "El cuerpo debe ser JSON.")
		return
	}

	s.mu.Lock()
	p1, p2 := s.teamByID(req.Player1), s.teamByID(req.Player2)
	if p1 == nil || p2 == nil || p1 == p2 {
		s.mu.Unlock()
		s.writeError(w, http.StatusBadRequest, "validation", "player1 y player2 deben ser dos equipos distintos ya registrados (GET /api/me).")
		return
	}
	now := s.now()
	g := &mockGame{kind: "friendly", toMove: 1, availableAt: now, startedAt: now}
	g.players[1], g.players[2] = p1, p2
	for _, c := range req.Opening {
		if c < 0 || c >= game.Cols {
			s.mu.Unlock()
			s.writeError(w, http.StatusBadRequest, "validation", fmt.Sprintf("opening: columna %d fuera de rango.", c))
			return
		}
		if _, err := g.play(now, g.toMove, game.Column(c), 0); err != nil || !g.playing() {
			s.mu.Unlock()
			s.writeError(w, http.StatusBadRequest, "validation", "opening: jugada ilegal o que termina la partida.")
			return
		}
	}
	s.created++
	g.id = fmt.Sprintf("g_f_%02d", s.created)
	s.games[g.id] = g
	s.advance(g, now) // avisa al equipo que tiene el primer turno
	s.mu.Unlock()

	s.writeJSON(w, http.StatusCreated, map[string]string{"game_id": g.id})
}

// teamByID busca un equipo por su id. Requiere s.mu tomado.
func (s *server) teamByID(id string) *team {
	for _, t := range s.teams {
		if t.id == id {
			return t
		}
	}
	return nil
}

// ---- GET /api/me y PUT /api/me/submission ----

type meResponse struct {
	Team struct {
		ID      string   `json:"id"`
		Name    string   `json:"name"`
		Members []string `json:"members"`
	} `json:"team"`
	Standing struct {
		Position int `json:"position"`
		Played   int `json:"played"`
		Won      int `json:"won"`
		Drawn    int `json:"drawn"`
		Lost     int `json:"lost"`
		Points   int `json:"points"`
	} `json:"standing"`
	PendingTurns []string          `json:"pending_turns"`
	Submission   *submissionRecord `json:"submission"`
}

func (s *server) handleMe(w http.ResponseWriter, _ *http.Request, t *team) {
	s.mu.Lock()
	now := s.now()
	var resp meResponse
	resp.Team.ID, resp.Team.Name, resp.Team.Members = t.id, t.name, []string{}
	resp.PendingTurns = []string{}
	for _, g := range s.games {
		g.expireAbsent(now)
		if g.pendingFor(t) {
			resp.PendingTurns = append(resp.PendingTurns, g.id)
		}
	}
	slices.Sort(resp.PendingTurns)
	if t.submission != nil {
		sub := *t.submission // copia, para leerla fuera del mutex
		resp.Submission = &sub
	}
	s.mu.Unlock()
	s.writeJSON(w, http.StatusOK, resp)
}

func (s *server) handleSubmission(w http.ResponseWriter, r *http.Request, t *team) {
	var req struct {
		RepoURL string `json:"repo_url"`
		Commit  string `json:"commit"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "validation", "El cuerpo debe ser JSON.")
		return
	}
	u, err := url.Parse(req.RepoURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		s.writeError(w, http.StatusBadRequest, "validation", "repo_url debe ser una URL http(s).")
		return
	}
	if len(req.Commit) < 7 || len(req.Commit) > 40 {
		s.writeError(w, http.StatusBadRequest, "validation", "commit debe tener entre 7 y 40 caracteres.")
		return
	}

	s.mu.Lock()
	rec := submissionRecord{RepoURL: req.RepoURL, Commit: req.Commit, RegisteredAt: formatTime(s.now())}
	t.submission = &rec
	s.mu.Unlock()
	s.writeJSON(w, http.StatusOK, rec)
}

// ---- Públicos ----

type playerInfo struct {
	TeamID string `json:"team_id"` // el ejemplo del OpenAPI usa team_id (el schema Team dice id)
	Name   string `json:"name"`
}

type gameResponse struct {
	GameID     string                `json:"game_id"`
	MatchID    *string               `json:"match_id"`
	Kind       string                `json:"kind"`
	Status     string                `json:"status"`
	Players    map[string]playerInfo `json:"players"`
	Moves      []moveRecord          `json:"moves"`
	Board      game.Board            `json:"board"`
	Result     *gameResult           `json:"result"`
	StartedAt  string                `json:"started_at"`
	FinishedAt *string               `json:"finished_at"`
}

func (s *server) handleGame(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("game_id") // valor de {game_id} en la ruta (Go 1.22+)
	s.mu.Lock()
	g := s.games[id]
	if g == nil {
		s.mu.Unlock()
		s.writeError(w, http.StatusNotFound, "unknown_game", fmt.Sprintf("La partida %s no existe.", id))
		return
	}
	g.expireAbsent(s.now())
	resp := gameResponse{
		GameID:    g.id,
		Kind:      g.kind,
		Status:    "playing",
		Players:   map[string]playerInfo{},
		Moves:     slices.Clone(g.moves), // copia: g.moves puede crecer mientras se serializa la respuesta
		Board:     g.board,
		StartedAt: formatTime(g.startedAt),
	}
	if resp.Moves == nil {
		resp.Moves = []moveRecord{}
	}
	for p := game.Player(1); p <= 2; p++ {
		name := s.nameOf(g, p)
		id := name
		if g.players[p] != nil {
			id = g.players[p].id
		}
		resp.Players[strconv.Itoa(int(p))] = playerInfo{TeamID: id, Name: name}
	}
	if !g.playing() {
		res := *g.result
		resp.Result = &res
		resp.Status = "finished"
		f := formatTime(g.finishedAt)
		resp.FinishedAt = &f
	}
	s.mu.Unlock()
	s.writeJSON(w, http.StatusOK, resp)
}

func (s *server) handleStandings(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]any{"phase": "practice", "standings": []any{}})
}

// ---- Helpers ----

// nameOf es el nombre del jugador side: el del equipo, o el del rival de la casa.
func (s *server) nameOf(g *mockGame, side game.Player) string {
	if t := g.players[side]; t != nil {
		return t.name
	}
	return g.house
}

func columns(cs []game.Column) []int {
	out := make([]int, len(cs))
	for i, c := range cs {
		out[i] = int(c)
	}
	return out
}

// formatTime usa el formato del OpenAPI: "2026-11-05T15:04:05.123Z".
func formatTime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z07:00")
}

func (s *server) writeJSON(w http.ResponseWriter, status int, v any) {
	if s.latency > 0 {
		time.Sleep(s.latency) // fuera del mutex: la demora no bloquea a los otros equipos
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write response", "err", err)
	}
}

func (s *server) writeError(w http.ResponseWriter, status int, code, message string) {
	s.writeJSON(w, status, struct {
		OK      bool   `json:"ok"`
		Error   string `json:"error"`
		Message string `json:"message"`
	}{false, code, message})
}
