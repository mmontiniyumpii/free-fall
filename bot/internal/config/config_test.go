package config

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// env arma un getenv falso a partir de un mapa, para no tocar el entorno real del proceso.
func env(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(env(nil))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := Config{
		Think:       1500 * time.Millisecond,
		Port:        "3000",
		PollWait:    20 * time.Second,
		TTEnabled:   false,
		MockLatency: 0,
	}
	if cfg != want {
		t.Errorf("cfg = %+v, want %+v", cfg, want)
	}
}

func TestLoadValues(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"ARBITRO_URL":     "http://localhost:4000",
		"TEAM_TOKEN":      "secret-token",
		"THINK_MS":        "1200",
		"PORT":            "3100",
		"POLL_WAIT":       "25",
		"TT_ENABLED":      "true",
		"MOCK_LATENCY_MS": "300",
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := Config{
		ArbiterURL:  "http://localhost:4000",
		TeamToken:   "secret-token",
		Think:       1200 * time.Millisecond,
		Port:        "3100",
		PollWait:    25 * time.Second,
		TTEnabled:   true,
		MockLatency: 300 * time.Millisecond,
	}
	if cfg != want {
		t.Errorf("cfg = %+v, want %+v", cfg, want)
	}
}

func TestLoadInvalid(t *testing.T) {
	tests := []struct {
		name, key, value string
	}{
		{"think not a number", "THINK_MS", "abc"},
		{"think zero", "THINK_MS", "0"},
		{"think reaches arbiter limit", "THINK_MS", "3000"},
		{"port not a number", "PORT", "http"},
		{"port out of range", "PORT", "70000"},
		{"poll wait negative", "POLL_WAIT", "-1"},
		{"poll wait above arbiter max", "POLL_WAIT", "26"},
		{"tt not a bool", "TT_ENABLED", "yes please"},
		{"mock latency negative", "MOCK_LATENCY_MS", "-5"},
		{"arbiter url without scheme", "ARBITRO_URL", "localhost:4000"},
		{"arbiter url with other scheme", "ARBITRO_URL", "ftp://arbitro"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			vars := map[string]string{tc.key: tc.value}
			if tc.key == "ARBITRO_URL" {
				vars["TEAM_TOKEN"] = "tok" // para que el único error posible sea la URL
			}
			_, err := Load(env(vars))
			if err == nil {
				t.Fatalf("Load(%s=%q) = nil error, want error", tc.key, tc.value)
			}
			if !strings.Contains(err.Error(), tc.key) {
				t.Errorf("error %q does not name the variable %s", err, tc.key)
			}
		})
	}
}

// Con árbitro configurado y sin token, el bot arrancaría y recibiría 401 para siempre: mejor no arrancar.
func TestArbiterRequiresToken(t *testing.T) {
	_, err := Load(env(map[string]string{"ARBITRO_URL": "http://localhost:4000"}))
	if err == nil || !strings.Contains(err.Error(), "TEAM_TOKEN") {
		t.Errorf("err = %v, want error que nombre TEAM_TOKEN", err)
	}
	// Sin árbitro (solo API local), el token no hace falta.
	if _, err := Load(env(nil)); err != nil {
		t.Errorf("sin ARBITRO_URL: err = %v, want nil", err)
	}
}

// El token nunca debe aparecer al imprimir o loguear la configuración.
func TestTokenNeverPrinted(t *testing.T) {
	cfg := Config{TeamToken: "secret-token", Port: "3000"}
	for _, format := range []string{"%v", "%+v", "%s"} {
		if out := fmt.Sprintf(format, cfg); strings.Contains(out, "secret-token") {
			t.Errorf("Sprintf(%q) leaks token: %s", format, out)
		}
	}
	if out := cfg.LogValue().String(); strings.Contains(out, "secret-token") {
		t.Errorf("LogValue leaks token: %s", out)
	}
}
