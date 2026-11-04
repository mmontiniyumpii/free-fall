package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mmontiniyumpii/free-fall/bot/internal/game"
)

// Server es la API local para la app web (docs/api-local.md). Las partidas viven en memoria.
type Server struct {
	mu     sync.Mutex // protege games y nextID; cada partida tiene además su propio mutex
	games  map[string]*localGame
	nextID int

	choose    Chooser
	think     time.Duration
	connected *atomic.Bool // lo escribe el loop del árbitro; /health lo lee
}

func NewServer(choose Chooser, think time.Duration, connected *atomic.Bool) *Server {
	return &Server{games: map[string]*localGame{}, choose: choose, think: think, connected: connected}
}

// Handler devuelve las rutas con CORS para cualquier origen.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	// Desde Go 1.22 el patrón incluye el método y variables de ruta como {id}.
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("POST /local/games", s.handleCreate)
	mux.HandleFunc("POST /local/games/{id}/moves", s.handleMove)
	mux.HandleFunc("GET /local/games/{id}", s.handleGet)
	return cors(mux)
}

// cors agrega los headers para que la app en :4200 pueda llamar a la API en :3000,
// y responde el preflight OPTIONS que manda el navegador antes de un POST con JSON.
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, struct {
		OK               bool `json:"ok"`
		ArbiterConnected bool `json:"arbiter_connected"`
		ThinkMS          int  `json:"think_ms"`
	}{true, s.connected.Load(), int(s.think / time.Millisecond)})
}

// POST /local/games {"first": "human" | "bot", "difficulty"?: "easy" | "medium" | "hard"}
func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		First      *string `json:"first"` // puntero: distingue "no vino" de ""
		Difficulty *string `json:"difficulty"`
	}
	if err := decode(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "validation", `El cuerpo debe ser {"first": "human" | "bot", "difficulty"?: "easy" | "medium" | "hard"}.`)
		return
	}
	if req.First == nil || (*req.First != "human" && *req.First != "bot") {
		writeError(w, http.StatusBadRequest, "validation", `first debe ser "human" o "bot".`)
		return
	}
	difficulty := "hard"
	if req.Difficulty != nil {
		if _, ok := maxDepthFor[*req.Difficulty]; !ok {
			writeError(w, http.StatusBadRequest, "validation", `difficulty debe ser "easy", "medium" o "hard".`)
			return
		}
		difficulty = *req.Difficulty
	}

	g := &localGame{difficulty: difficulty, status: "playing", toMove: 1, human: 1, bot: 2}
	if *req.First == "bot" {
		g.human, g.bot = 2, 1
	}
	s.mu.Lock()
	s.nextID++
	g.id = fmt.Sprintf("l_%04d", s.nextID)
	s.games[g.id] = g
	s.mu.Unlock()

	// Se toma el mutex de la partida antes de que nadie más la pueda ver por GET.
	g.mu.Lock()
	if g.bot == 1 {
		g.botPlay(s.choose) // la respuesta ya incluye la apertura del bot
	}
	st := g.state()
	g.mu.Unlock()
	writeJSON(w, http.StatusCreated, st)
}

// POST /local/games/{id}/moves {"column": 0-7}
func (s *Server) handleMove(w http.ResponseWriter, r *http.Request) {
	g, ok := s.lookup(w, r)
	if !ok {
		return
	}
	var req struct {
		Column *int `json:"column"` // puntero: column 0 es válida, hay que distinguirla de "no vino"
	}
	if err := decode(w, r, &req); err != nil || req.Column == nil {
		writeError(w, http.StatusBadRequest, "validation", `El cuerpo debe ser {"column": entero entre 0 y 7}.`)
		return
	}
	col := *req.Column

	g.mu.Lock()
	defer g.mu.Unlock() // defer: se libera al salir, por cualquier return; la respuesta se escribe con el mutex tomado
	switch {
	case g.status != "playing":
		writeError(w, http.StatusConflict, "game_over", fmt.Sprintf("La partida %s ya terminó.", g.id))
		return
	case g.toMove != g.human:
		writeError(w, http.StatusConflict, "not_your_turn", "No es el turno del humano.")
		return
	case col < 0 || col >= game.Cols:
		writeError(w, http.StatusBadRequest, "illegal_move", fmt.Sprintf("La columna %d está fuera de rango.", col))
		return
	}
	if err := g.apply(game.Column(col), g.human); err != nil {
		if errors.Is(err, game.ErrColumnFull) {
			writeError(w, http.StatusBadRequest, "illegal_move", fmt.Sprintf("La columna %d está llena.", col))
			return
		}
		slog.Error("human move rejected by rules", "game_id", g.id, "column", col, "err", err)
		writeError(w, http.StatusBadRequest, "illegal_move", err.Error())
		return
	}
	if g.status == "playing" {
		g.botPlay(s.choose) // si el humano no cerró la partida, el bot responde en la misma llamada
	}
	writeJSON(w, http.StatusOK, g.state())
}

// GET /local/games/{id}
func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	g, ok := s.lookup(w, r)
	if !ok {
		return
	}
	g.mu.Lock()
	st := g.state()
	g.mu.Unlock()
	writeJSON(w, http.StatusOK, st)
}

// lookup busca la partida de {id}; si no existe responde 404 unknown_game.
func (s *Server) lookup(w http.ResponseWriter, r *http.Request) (*localGame, bool) {
	id := r.PathValue("id")
	s.mu.Lock()
	g := s.games[id]
	s.mu.Unlock()
	if g == nil {
		writeError(w, http.StatusNotFound, "unknown_game", fmt.Sprintf("La partida %s no existe.", id))
		return nil, false
	}
	return g, true
}

func decode(w http.ResponseWriter, r *http.Request, v any) error {
	return json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(v)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write response", "err", err)
	}
}

// writeError usa el mismo formato de error que el árbitro, a propósito.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, struct {
		OK      bool   `json:"ok"`
		Error   string `json:"error"`
		Message string `json:"message"`
	}{false, code, message})
}
