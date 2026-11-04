package arbiter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mmontiniyumpii/free-fall/bot/internal/game"
)

// APIError es una respuesta no-200 del árbitro, con el código de error del OpenAPI si vino en el cuerpo.
type APIError struct {
	Status  int
	Code    string // "illegal_move", "timeout", "not_your_turn", "unauthorized", ... o "" si el cuerpo no era JSON
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("arbiter: HTTP %d %s: %s", e.Status, e.Code, e.Message)
}

// Client habla con el árbitro. El token solo viaja en el header Authorization: nunca se loguea ni va en un error.
type Client struct {
	base  string
	token string
	poll  *http.Client // GET /api/next: el long poll dura hasta 25 s
	move  *http.Client // POST /api/move: tiene que ser rápido
}

// NewClient crea el cliente con los timeouts de AGENTS.md: 35 s para el long poll y 5 s para la jugada.
func NewClient(baseURL, token string) *Client {
	return newClient(baseURL, token, 35*time.Second, 5*time.Second)
}

func newClient(baseURL, token string, pollTimeout, moveTimeout time.Duration) *Client {
	return &Client{
		base:  strings.TrimRight(baseURL, "/"),
		token: token,
		// Timeout: si el servidor no termina de responder en ese plazo, Do devuelve error en vez de colgarse.
		poll: &http.Client{Timeout: pollTimeout},
		move: &http.Client{Timeout: moveTimeout},
	}
}

// Next pide el turno pendiente (long poll). ok = false significa idle.
func (c *Client) Next(ctx context.Context, wait time.Duration) (Turn, bool, error) {
	path := "/api/next?wait=" + strconv.Itoa(int(wait/time.Second))
	data, err := c.do(ctx, c.poll, http.MethodGet, path, nil)
	if err != nil {
		return Turn{}, false, err
	}
	return ParseNext(data)
}

// Move envía la jugada de un turno pendiente.
func (c *Client) Move(ctx context.Context, gameID string, col game.Column) (MoveResult, error) {
	body, err := json.Marshal(struct {
		GameID string `json:"game_id"`
		Column int    `json:"column"` // int y no game.Column, para que el JSON sea un número
	}{gameID, int(col)})
	if err != nil {
		return MoveResult{}, err
	}
	data, err := c.do(ctx, c.move, http.MethodPost, "/api/move", body)
	if err != nil {
		return MoveResult{}, err
	}
	return parseMove(data)
}

func (c *Client) do(ctx context.Context, hc *http.Client, method, path string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("arbiter %s %s: %w", method, req.URL.Path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("arbiter %s %s: read body: %w", method, req.URL.Path, err)
	}
	if resp.StatusCode != http.StatusOK {
		apiErr := &APIError{Status: resp.StatusCode}
		var e struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if json.Unmarshal(data, &e) == nil {
			apiErr.Code, apiErr.Message = e.Error, e.Message
		} else {
			apiErr.Message = truncate(string(data), 200)
		}
		return nil, apiErr
	}
	return data, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
