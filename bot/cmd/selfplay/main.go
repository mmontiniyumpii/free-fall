// Command selfplay enfrenta dos instancias del motor entre sí a través del mock del árbitro,
// con el mismo cliente y el mismo loop que usa el bot en el torneo (paso 7 de PLAN_BACKEND.md).
//
// Uso (con el mock corriendo, por ejemplo MOCK_LATENCY_MS=300 go run ./cmd/mock):
//
//	go run -race ./cmd/selfplay -duration 30m -latency 300ms
//
// Sale con código 1 si hubo errores, partidas perdidas por timeout/ilegal/ausencia, o jugadas más
// lentas que 1 800 ms + latencia simulada (docs/TECNICO.md, D5).
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/mmontiniyumpii/free-fall/bot/internal/arbiter"
)

func main() {
	base := flag.String("arbiter", "http://localhost:4000", "URL del mock del árbitro")
	duration := flag.Duration("duration", 30*time.Minute, "cuánto tiempo jugar (termina la partida en curso)")
	think := flag.Duration("think", 1500*time.Millisecond, "THINK_MS de los dos motores")
	latency := flag.Duration("latency", 300*time.Millisecond, "MOCK_LATENCY_MS con que corre el mock (para el criterio)")
	seed := flag.Uint64("seed", uint64(time.Now().UnixNano()), "semilla de las aperturas")
	flag.Parse() // flag.X devuelve punteros: *duration es el valor después de Parse

	// Ctrl+C corta la corrida y muestra el resumen de lo jugado hasta ahí.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, *base, *duration, *think, *latency, *seed, os.Stdout, os.Stderr))
}

func run(ctx context.Context, base string, duration, think, latency time.Duration, seed uint64, out, logs io.Writer) int {
	base = strings.TrimRight(base, "/")
	ctrl := &http.Client{Timeout: 10 * time.Second} // llamadas de control al mock: siempre con timeout
	tokens := [2]string{"selfplay-a", "selfplay-b"}
	var ids [2]string
	for i, tok := range tokens {
		id, err := teamID(ctx, ctrl, base, tok)
		if err != nil {
			fmt.Fprintf(out, "selfplay: no se pudo registrar el equipo %d en %s: %v\n", i+1, base, err)
			return 1
		}
		ids[i] = id
	}

	// Los dos loops del árbitro, uno por equipo, en sus goroutines; loopCtx los corta al final.
	var errs, warns atomic.Int64
	loopCtx, stopLoops := context.WithCancel(context.Background())
	var wg sync.WaitGroup // WaitGroup: espera a que los dos loops terminen antes de salir
	for i, tok := range tokens {
		handler := countingHandler{Handler: slog.NewTextHandler(logs, nil), errors: &errs, warns: &warns}
		logger := slog.New(handler).With("team", string(rune('A'+i)))
		wg.Add(1)
		go func() {
			defer wg.Done()
			arbiter.RunLoop(loopCtx, arbiter.NewClient(base, tok), arbiter.EngineChooser(think), logger,
				arbiter.LoopOptions{Wait: 2 * time.Second})
		}()
	}

	limit := 1800*time.Millisecond + latency
	sum := newSummary(limit)
	end := time.Now().Add(duration)
	start := time.Now()
	fmt.Fprintf(out, "selfplay: %v contra %s, think %v, latencia %v, límite por jugada %v, semilla %d\n",
		duration, base, think, latency, limit, seed)

	for n := 0; time.Now().Before(end) && ctx.Err() == nil; n++ {
		p1, p2 := ids[n%2], ids[(n+1)%2] // alterna quién abre
		opening := randomOpening(seed+uint64(n), 4)
		id, err := friendly(ctx, ctrl, base, p1, p2, opening)
		if err != nil {
			fmt.Fprintf(out, "selfplay: no se pudo crear la partida %d: %v\n", n+1, err)
			break
		}
		g, err := waitFinished(ctx, ctrl, base, id, 10*time.Minute)
		if err != nil {
			fmt.Fprintf(out, "selfplay: partida %s: %v\n", id, err)
			break
		}
		sum.add(g)
		fmt.Fprintf(out, "partida %3d %s abre=%s apertura=%v %s ganador=%s jugadas=%d ms_max=%d\n",
			n+1, id, string(rune('A'+n%2)), opening, g.Result.Reason, winnerLabel(g, n), len(g.Moves), maxMS(g))
	}

	stopLoops()
	wg.Wait()

	fmt.Fprintf(out, "\nRESUMEN (%v): %d partidas, resultados %v, victorias jugador1/jugador2 %d/%d\n",
		time.Since(start).Round(time.Second), sum.games, sum.reasons, sum.winsByPlayer[1], sum.winsByPlayer[2])
	fmt.Fprintf(out, "ms máximo %d (límite %v; el árbitro real pierde la partida a los 3000), jugadas lentas %d, logs ERROR %d, WARN %d\n",
		sum.maxMS, limit, sum.slowMoves, errs.Load(), warns.Load())
	if fails := sum.failures(errs.Load(), warns.Load()); len(fails) > 0 {
		fmt.Fprintf(out, "FALLA: %s\n", strings.Join(fails, "; "))
		return 1
	}
	fmt.Fprintln(out, "OK: sin errores ni jugadas fuera de límite")
	return 0
}

func winnerLabel(g gameRecord, n int) string {
	if g.Result == nil || g.Result.Winner == nil {
		return "-"
	}
	// El equipo que abre la partida n es A si n es par: el jugador 1 es A en las pares y B en las impares.
	if (*g.Result.Winner == 1) == (n%2 == 0) {
		return "A"
	}
	return "B"
}

func maxMS(g gameRecord) int64 {
	var m int64
	for _, mv := range g.Moves {
		m = max(m, mv.MS)
	}
	return m
}

// ---- Llamadas de control al mock ----

func teamID(ctx context.Context, c *http.Client, base, token string) (string, error) {
	var me struct {
		Team struct {
			ID string `json:"id"`
		} `json:"team"`
	}
	if err := doJSON(ctx, c, http.MethodGet, base+"/api/me", token, nil, http.StatusOK, &me); err != nil {
		return "", err
	}
	return me.Team.ID, nil
}

func friendly(ctx context.Context, c *http.Client, base, p1, p2 string, opening []int) (string, error) {
	body := map[string]any{"player1": p1, "player2": p2, "opening": opening}
	var resp struct {
		GameID string `json:"game_id"`
	}
	if err := doJSON(ctx, c, http.MethodPost, base+"/mock/friendly", "", body, http.StatusCreated, &resp); err != nil {
		return "", err
	}
	return resp.GameID, nil
}

// waitFinished consulta la partida cada 250 ms hasta que termina.
func waitFinished(ctx context.Context, c *http.Client, base, id string, timeout time.Duration) (gameRecord, error) {
	deadline := time.Now().Add(timeout)
	for {
		var g gameRecord
		if err := doJSON(ctx, c, http.MethodGet, base+"/api/games/"+id, "", nil, http.StatusOK, &g); err != nil {
			return g, err
		}
		if g.Status == "finished" && g.Result != nil {
			return g, nil
		}
		if time.Now().After(deadline) {
			return g, fmt.Errorf("no terminó en %v (%d jugadas)", timeout, len(g.Moves))
		}
		select {
		case <-time.After(250 * time.Millisecond):
		case <-ctx.Done():
			return g, ctx.Err()
		}
	}
}

func doJSON(ctx context.Context, c *http.Client, method, url, token string, body any, want int, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != want {
		return errors.New(resp.Status + ": " + strings.TrimSpace(string(data)))
	}
	return json.Unmarshal(data, out)
}
