package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const mod = "github.com/mmontiniyumpii/free-fall/bot"

// profile arma un coverage.out con el formato de `go test -coverprofile`:
// "archivo:inicio,fin sentencias veces_ejecutado".
func profile(lines ...string) string {
	return "mode: atomic\n" + strings.Join(lines, "\n") + "\n"
}

func TestRun(t *testing.T) {
	tests := []struct {
		name     string
		profile  string
		wantCode int
		wantOut  string
	}{
		{
			name: "both packages above threshold",
			profile: profile(
				mod+"/internal/game/rules.go:1.1,3.2 7 1",
				mod+"/internal/game/rules.go:4.1,5.2 3 0",
				mod+"/internal/engine/search.go:1.1,2.2 8 4",
				mod+"/internal/engine/search.go:3.1,4.2 2 0",
			),
			wantCode: 0,
			wantOut:  "internal/game: 70.0%",
		},
		{
			name: "one package below threshold fails even if combined is above",
			profile: profile(
				mod+"/internal/game/rules.go:1.1,3.2 6 1",
				mod+"/internal/game/rules.go:4.1,5.2 4 0",
				mod+"/internal/engine/search.go:1.1,2.2 90 1",
			),
			wantCode: 1,
			wantOut:  "internal/game: 60.0%",
		},
		{
			name: "packages outside the check are ignored",
			profile: profile(
				mod+"/internal/game/rules.go:1.1,3.2 1 1",
				mod+"/internal/engine/search.go:1.1,2.2 1 1",
				mod+"/internal/api/server.go:1.1,2.2 50 0",
			),
			wantCode: 0,
		},
		{
			name: "subpackage of game is not counted as game",
			profile: profile(
				mod+"/internal/game/rules.go:1.1,3.2 1 1",
				mod+"/internal/game/extra/x.go:1.1,3.2 50 0",
				mod+"/internal/engine/search.go:1.1,2.2 1 1",
			),
			wantCode: 0,
		},
		{
			name: "duplicate block counts once and as covered if any run hit it",
			profile: profile(
				mod+"/internal/game/rules.go:1.1,3.2 10 0",
				mod+"/internal/game/rules.go:1.1,3.2 10 2",
				mod+"/internal/engine/search.go:1.1,2.2 1 1",
			),
			wantCode: 0,
			wantOut:  "internal/game: 100.0% (10/10",
		},
		{
			name:     "no statements yet only warns",
			profile:  profile(mod + "/cmd/bot/main.go:1.1,2.2 5 1"),
			wantCode: 0,
			wantOut:  "internal/game: sin sentencias todavía",
		},
		{
			name:     "malformed line",
			profile:  profile("garbage"),
			wantCode: 2,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "coverage.out")
			if err := os.WriteFile(path, []byte(tc.profile), 0o600); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			code := run([]string{path}, &out)
			if code != tc.wantCode {
				t.Errorf("exit code = %d, want %d\noutput:\n%s", code, tc.wantCode, out.String())
			}
			if tc.wantOut != "" && !strings.Contains(out.String(), tc.wantOut) {
				t.Errorf("output does not contain %q:\n%s", tc.wantOut, out.String())
			}
		})
	}
}

func TestRunMissingFile(t *testing.T) {
	var out bytes.Buffer
	if code := run([]string{filepath.Join(t.TempDir(), "nope.out")}, &out); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}

func TestRunUsage(t *testing.T) {
	var out bytes.Buffer
	if code := run(nil, &out); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}
