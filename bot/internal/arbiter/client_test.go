package arbiter

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

const token = "secret-team-token"

// respond arma un handler que siempre contesta status y body.
func respond(status int, body []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write(body)
	}
}

func TestNewClientHasTimeouts(t *testing.T) {
	c := NewClient("http://x", token)
	// Sin timeout, un socket colgado deja al bot mudo y se pierde la partida por ausencia.
	if c.poll.Timeout != 35*time.Second || c.move.Timeout != 5*time.Second {
		t.Errorf("timeouts = %v / %v, want 35s / 5s", c.poll.Timeout, c.move.Timeout)
	}
}

// captured es lo que el handler vio del request; viaja al test por un canal.
type captured struct {
	method, path, wait, auth, contentType string
	body                                  map[string]any
}

// recorder responde siempre status/body y manda cada request recibido por el canal.
func recorder(status int, body []byte) (http.HandlerFunc, chan captured) {
	ch := make(chan captured, 10)
	return func(w http.ResponseWriter, r *http.Request) {
		c := captured{
			method: r.Method, path: r.URL.Path, wait: r.URL.Query().Get("wait"),
			auth: r.Header.Get("Authorization"), contentType: r.Header.Get("Content-Type"),
		}
		if data, err := io.ReadAll(r.Body); err == nil && len(data) > 0 {
			json.Unmarshal(data, &c.body) // si no es JSON, body queda nil y el test lo detecta
		}
		ch <- c
		respond(status, body)(w, r)
	}, ch
}

func TestNextRequest(t *testing.T) {
	handler, requests := recorder(200, readFixture(t, "next-turn.json"))
	ts := httptest.NewServer(handler)
	defer ts.Close()

	turn, ok, err := NewClient(ts.URL+"/", token).Next(context.Background(), 20*time.Second)
	if err != nil || !ok || turn.GameID != "g_7f3a" {
		t.Fatalf("Next = %+v, %v, %v", turn, ok, err)
	}
	got := <-requests
	if got.method != "GET" || got.path != "/api/next" || got.wait != "20" {
		t.Errorf("request = %s %s wait=%s", got.method, got.path, got.wait)
	}
	if got.auth != "Bearer "+token {
		t.Errorf("Authorization = %q", got.auth)
	}
	if got.contentType != "application/json" {
		t.Errorf("Content-Type = %q", got.contentType)
	}
}
func TestNextIdle(t *testing.T) {
	ts := httptest.NewServer(respond(200, readFixture(t, "next-idle.json")))
	defer ts.Close()
	if _, ok, err := NewClient(ts.URL, token).Next(context.Background(), 0); err != nil || ok {
		t.Errorf("Next(idle) = ok %v, err %v", ok, err)
	}
}

func TestMoveRequestAndResults(t *testing.T) {
	for _, file := range []string{"move-playing.json", "move-won.json", "move-draw.json"} {
		t.Run(file, func(t *testing.T) {
			fixture := readFixture(t, file)
			handler, requests := recorder(200, fixture)
			ts := httptest.NewServer(handler)
			defer ts.Close()

			res, err := NewClient(ts.URL, token).Move(context.Background(), "g_7f3a", 4)
			if err != nil {
				t.Fatal(err)
			}
			want, err := parseMove(fixture)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(res, want) {
				t.Errorf("Move = %+v, want %+v", res, want)
			}
			got := <-requests
			if got.method != "POST" || got.path != "/api/move" || got.auth != "Bearer "+token || got.contentType != "application/json" {
				t.Errorf("request = %+v", got)
			}
			if !reflect.DeepEqual(got.body, map[string]any{"game_id": "g_7f3a", "column": 4.0}) {
				t.Errorf("body = %v, want exactamente game_id y column", got.body)
			}
		})
	}
}

func TestAPIErrors(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     []byte
		wantCode string
	}{
		{"illegal move", 400, readFixture(t, "error-illegal-move.json"), "illegal_move"},
		{"timeout", 400, readFixture(t, "error-timeout.json"), "timeout"},
		{"not your turn", 409, readFixture(t, "error-not-your-turn.json"), "not_your_turn"},
		{"unauthorized", 401, readFixture(t, "error-unauthorized.json"), "unauthorized"},
		{"server error without json", 502, []byte("<html>Bad Gateway</html>"), ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(respond(tc.status, tc.body))
			defer ts.Close()
			c := NewClient(ts.URL, token)

			_, err := c.Move(context.Background(), "g_7f3a", 3)
			var apiErr *APIError // errors.As busca un *APIError dentro de la cadena de errores
			if !errors.As(err, &apiErr) {
				t.Fatalf("err = %v, want *APIError", err)
			}
			if apiErr.Status != tc.status || apiErr.Code != tc.wantCode {
				t.Errorf("APIError = %+v, want status %d code %q", apiErr, tc.status, tc.wantCode)
			}
			if _, _, err := c.Next(context.Background(), 0); !errors.As(err, &apiErr) || apiErr.Status != tc.status {
				t.Errorf("Next: err = %v, want *APIError %d", err, tc.status)
			}
		})
	}
}

func TestMalformedTurnIsAnError(t *testing.T) {
	ts := httptest.NewServer(respond(200, []byte(`{"status":"turn","game_id":"g"}`)))
	defer ts.Close()
	if _, _, err := NewClient(ts.URL, token).Next(context.Background(), 0); !errors.Is(err, ErrMalformed) {
		t.Errorf("err = %v, want ErrMalformed", err)
	}
}

func TestClientTimeoutAndCancel(t *testing.T) {
	// El servidor no responde nunca: se queda esperando hasta que el cliente corta.
	// Lee el cuerpo primero: net/http solo detecta que el cliente cortó (y cancela r.Context())
	// después de que el handler terminó de leer el cuerpo del request.
	release := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-release: // vía de escape para que ts.Close no quede esperando
		}
	}))
	defer ts.Close()
	defer close(release) // los defer corren en orden inverso: primero se libera el handler, después Close

	c := newClient(ts.URL, token, 200*time.Millisecond, 100*time.Millisecond)
	start := time.Now()
	if _, err := c.Move(context.Background(), "g", 0); err == nil {
		t.Fatal("Move sin respuesta: err = nil, want timeout")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("el timeout de Move tardó %v", elapsed)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start = time.Now()
	if _, _, err := NewClient(ts.URL, token).Next(ctx, 20*time.Second); err == nil {
		t.Fatal("Next con contexto cancelado: err = nil")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("cancelar el contexto tardó %v en cortar el long poll", elapsed)
	}
}

func TestErrorsNeverContainToken(t *testing.T) {
	ts := httptest.NewServer(respond(401, readFixture(t, "error-unauthorized.json")))
	c := NewClient(ts.URL, token)
	_, _, err401 := c.Next(context.Background(), 0)
	ts.Close()
	_, errNet := c.Move(context.Background(), "g", 0) // servidor cerrado: error de red

	for _, err := range []error{err401, errNet} {
		if err == nil {
			t.Fatal("se esperaba un error")
		}
		if strings.Contains(err.Error(), token) {
			t.Errorf("el error expone el token: %v", err)
		}
	}
}
