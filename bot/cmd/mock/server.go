package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"regexp"
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
	// Públicos: sin token. /api/state, /api/live y /api/openapi.json no están en el mock (docs/TECNICO.md, D13).
	mux.HandleFunc("GET /api/games", s.handleGames)
	mux.HandleFunc("GET /api/games/{game_id}", s.handleGame)
	mux.HandleFunc("GET /api/standings", s.handleStandings)
	mux.HandleFunc("GET /api/health", s.handleHealth)
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
		MatchID:    g.matchID,
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
	if g == nil {
		s.mu.Unlock()
		s.writeError(w, http.StatusConflict, "unknown_game", fmt.Sprintf("La partida %s no existe.", *req.GameID))
		return
	}
	if !g.pendingFor(t) {
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
		g.forfeit(now, side, errCode) // jugada rechazada: la partida se pierde
		ms := elapsed.Milliseconds()
		g.result.MS = &ms
		if errCode == "illegal_move" {
			g.result.Column = &col
		}
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
			g.forfeit(now, side, "illegal_move")
		}
	}
}

// ---- POST /api/practice ----

func (s *server) handlePractice(w http.ResponseWriter, r *http.Request, t *team) {
	var req struct {
		Opponent *string `json:"opponent"` // puntero: si falta, el OpenAPI dice que es casa-basico
		First    *int    `json:"first"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "validation", "El cuerpo debe ser JSON.")
		return
	}
	opponent := houseBasic
	if req.Opponent != nil {
		opponent = *req.Opponent
	}
	if opponent != houseRandom && opponent != houseBasic {
		s.writeError(w, http.StatusBadRequest, "validation", fmt.Sprintf("Rival inválido: %s.", opponent))
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
		seq:         s.created,
		kind:        "practice",
		house:       opponent,
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
	friendly := "amistoso" // match_id de los amistosos según el OpenAPI 1.0.1
	g := &mockGame{kind: "friendly", matchID: &friendly, toMove: 1, availableAt: now, startedAt: now}
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
	g.seq = s.created
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
	Phase        string            `json:"phase"`
}

// phase es la fase del torneo. El mock siempre está en prácticas: no hay liga ni playoffs.
const phase = "practice"

func (s *server) handleMe(w http.ResponseWriter, _ *http.Request, t *team) {
	s.mu.Lock()
	now := s.now()
	var resp meResponse
	resp.Team.ID, resp.Team.Name, resp.Team.Members = t.id, t.name, []string{}
	resp.PendingTurns = []string{}
	resp.Phase = phase
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

// Patrones del schema Submission del OpenAPI. MustCompile entra en panic si la expresión es inválida:
// como son constantes, eso solo puede pasar al arrancar, nunca en medio de un pedido.
var (
	repoURLPattern = regexp.MustCompile(`^https?://(github\.com|bitbucket\.org)/.+`)
	commitPattern  = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)
)

func (s *server) handleSubmission(w http.ResponseWriter, r *http.Request, t *team) {
	var req struct {
		RepoURL string `json:"repo_url"`
		Commit  string `json:"commit"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "validation", "El cuerpo debe ser JSON.")
		return
	}
	if !repoURLPattern.MatchString(req.RepoURL) {
		s.writeError(w, http.StatusBadRequest, "validation", "repo_url debe ser un repositorio de github.com o bitbucket.org.")
		return
	}
	if !commitPattern.MatchString(req.Commit) {
		s.writeError(w, http.StatusBadRequest, "validation", "commit debe ser un hash hexadecimal de 7 a 40 caracteres.")
		return
	}

	s.mu.Lock()
	rec := submissionRecord{RepoURL: req.RepoURL, Commit: req.Commit, RegisteredAt: formatTime(s.now())}
	t.submission = &rec
	s.mu.Unlock()
	s.writeJSON(w, http.StatusOK, rec)
}

// ---- Públicos ----

// playerInfo es el schema PlayerRef del OpenAPI.
type playerInfo struct {
	TeamID string `json:"team_id"`
	Name   string `json:"name"`
	House  bool   `json:"house,omitempty"` // solo aparece (en true) para el bot de la casa
}

// gameSummary es el schema GameSummary: una partida sin el tablero, como la lista GET /api/games.
type gameSummary struct {
	GameID     string                `json:"game_id"`
	MatchID    *string               `json:"match_id"`
	Kind       string                `json:"kind"`
	Series     *string               `json:"series"` // siempre null: el mock no tiene playoffs
	Status     string                `json:"status"`
	Players    map[string]playerInfo `json:"players"`
	Moves      []moveRecord          `json:"moves"`
	Current    *game.Player          `json:"current"` // a quién le toca; null si la partida terminó
	Result     *gameResult           `json:"result"`
	StartedAt  string                `json:"started_at"`
	FinishedAt *string               `json:"finished_at"`
}

// gameResponse es el schema Game: el resumen más el tablero. gameSummary va embebido (sin nombre de campo),
// así que encoding/json escribe sus campos al mismo nivel que board, igual que el allOf del OpenAPI.
type gameResponse struct {
	gameSummary
	Board game.Board `json:"board"`
}

// summaryOf arma el resumen de g. Requiere s.mu tomado.
func (s *server) summaryOf(g *mockGame) gameSummary {
	sum := gameSummary{
		GameID:    g.id,
		MatchID:   g.matchID,
		Kind:      g.kind,
		Status:    "playing",
		Players:   map[string]playerInfo{},
		Moves:     slices.Clone(g.moves), // copia: g.moves puede crecer mientras se serializa la respuesta
		StartedAt: formatTime(g.startedAt),
	}
	if sum.Moves == nil {
		sum.Moves = []moveRecord{}
	}
	for p := game.Player(1); p <= 2; p++ {
		if t := g.players[p]; t != nil {
			sum.Players[strconv.Itoa(int(p))] = playerInfo{TeamID: t.id, Name: t.name}
		} else {
			sum.Players[strconv.Itoa(int(p))] = playerInfo{TeamID: g.house, Name: g.house, House: true}
		}
	}
	if g.playing() {
		toMove := g.toMove
		sum.Current = &toMove
	} else {
		res := *g.result
		sum.Result = &res
		sum.Status = "finished"
		f := formatTime(g.finishedAt)
		sum.FinishedAt = &f
	}
	return sum
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
	resp := gameResponse{gameSummary: s.summaryOf(g), Board: g.board}
	s.mu.Unlock()
	s.writeJSON(w, http.StatusOK, resp)
}

// gameKinds son los valores de kind que acepta GET /api/games.
var gameKinds = []string{"practice", "friendly", "league", "third_place", "final"}

// handleGames lista las partidas sin tablero, de la más reciente a la más vieja.
func (s *server) handleGames(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	if kind != "" && !slices.Contains(gameKinds, kind) {
		s.writeError(w, http.StatusBadRequest, "validation", "kind debe ser practice, friendly, league, third_place o final.")
		return
	}
	limit := 50
	if q := r.URL.Query().Get("limit"); q != "" {
		n, err := strconv.Atoi(q)
		if err != nil || n < 1 || n > 500 {
			s.writeError(w, http.StatusBadRequest, "validation", "limit debe ser un entero entre 1 y 500.")
			return
		}
		limit = n
	}

	s.mu.Lock()
	now := s.now()
	var games []*mockGame
	for _, g := range s.games {
		if kind == "" || g.kind == kind {
			g.expireAbsent(now)
			games = append(games, g)
		}
	}
	// SortFunc ordena con una función de comparación: negativo = a va antes que b. Acá, mayor seq primero.
	slices.SortFunc(games, func(a, b *mockGame) int { return b.seq - a.seq })
	list := []gameSummary{} // no nil: sin partidas se responde [] y no null
	for _, g := range games[:min(limit, len(games))] {
		list = append(list, s.summaryOf(g))
	}
	s.mu.Unlock()
	s.writeJSON(w, http.StatusOK, list)
}

func (s *server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "phase": phase, "time": formatTime(s.now())})
}

func (s *server) handleStandings(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]any{"phase": phase, "standings": []any{}})
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
