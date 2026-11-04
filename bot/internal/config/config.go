// Package config lee la configuración del bot desde variables de entorno.
// Ver .env.example en la raíz del repositorio.
package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"time"
)

// Config es la configuración ya validada. El token nunca se imprime: ver String y LogValue.
type Config struct {
	ArbiterURL  string        // ARBITRO_URL; vacío = solo API local, sin loop del árbitro
	TeamToken   string        // TEAM_TOKEN
	Think       time.Duration // THINK_MS: presupuesto de cómputo por jugada
	Port        string        // PORT: puerto de la API local
	PollWait    time.Duration // POLL_WAIT: segundos del long poll a GET /api/next
	TTEnabled   bool          // TT_ENABLED: tabla de transposición (fase 2)
	MockLatency time.Duration // MOCK_LATENCY_MS: latencia artificial del mock
}

// arbiterLimit es el límite del árbitro por jugada; un presupuesto igual o mayor pierde siempre por tiempo.
const arbiterLimit = 3000 * time.Millisecond

// Load arma la Config. Recibe getenv (os.Getenv en producción) para poder probarse sin tocar el entorno real.
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		ArbiterURL: getenv("ARBITRO_URL"),
		TeamToken:  getenv("TEAM_TOKEN"),
		Port:       "3000",
	}

	if cfg.ArbiterURL != "" {
		u, err := url.Parse(cfg.ArbiterURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return Config{}, fmt.Errorf("ARBITRO_URL=%q: debe ser una URL http(s), por ejemplo http://localhost:4000", cfg.ArbiterURL)
		}
		if cfg.TeamToken == "" {
			return Config{}, fmt.Errorf("TEAM_TOKEN: es obligatorio cuando ARBITRO_URL está definido")
		}
	}

	thinkMS, err := intVar(getenv, "THINK_MS", 1500, 1, int(arbiterLimit/time.Millisecond)-1)
	if err != nil {
		return Config{}, err
	}
	cfg.Think = time.Duration(thinkMS) * time.Millisecond

	port, err := intVar(getenv, "PORT", 3000, 1, 65535)
	if err != nil {
		return Config{}, err
	}
	cfg.Port = strconv.Itoa(port)

	// El OpenAPI del árbitro acepta wait entre 0 y 25 segundos.
	pollWait, err := intVar(getenv, "POLL_WAIT", 20, 0, 25)
	if err != nil {
		return Config{}, err
	}
	cfg.PollWait = time.Duration(pollWait) * time.Second

	if raw := getenv("TT_ENABLED"); raw != "" {
		cfg.TTEnabled, err = strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("TT_ENABLED=%q: debe ser true o false", raw)
		}
	}

	latencyMS, err := intVar(getenv, "MOCK_LATENCY_MS", 0, 0, 60000)
	if err != nil {
		return Config{}, err
	}
	cfg.MockLatency = time.Duration(latencyMS) * time.Millisecond

	return cfg, nil
}

// intVar lee un entero en [min, max]; si la variable está vacía devuelve def.
func intVar(getenv func(string) string, key string, def, min, max int) (int, error) {
	raw := getenv(key)
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s=%q: no es un número entero", key, raw)
	}
	if n < min || n > max {
		return 0, fmt.Errorf("%s=%d: debe estar entre %d y %d", key, n, min, max)
	}
	return n, nil
}

// String hace que fmt (%v, %+v, %s) imprima la Config sin el token.
// Al definir este método, Config cumple la interfaz fmt.Stringer y fmt lo usa automáticamente.
func (c Config) String() string {
	return fmt.Sprintf("{ArbiterURL:%s TeamToken:%s Think:%s Port:%s PollWait:%s TTEnabled:%t MockLatency:%s}",
		c.ArbiterURL, mask(c.TeamToken), c.Think, c.Port, c.PollWait, c.TTEnabled, c.MockLatency)
}

// LogValue hace lo mismo para log/slog (interfaz slog.LogValuer).
func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("arbiter_url", c.ArbiterURL),
		slog.String("team_token", mask(c.TeamToken)),
		slog.Duration("think", c.Think),
		slog.String("port", c.Port),
		slog.Duration("poll_wait", c.PollWait),
		slog.Bool("tt_enabled", c.TTEnabled),
		slog.Duration("mock_latency", c.MockLatency),
	)
}

func mask(token string) string {
	if token == "" {
		return "(vacío)"
	}
	return "(definido)"
}
