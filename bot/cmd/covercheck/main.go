// Command covercheck lee un perfil de cobertura (go test -coverprofile) y falla
// si internal/game o internal/engine quedan por debajo del 70 % de sentencias cubiertas.
//
// Uso: go run ./cmd/covercheck coverage.out
// Códigos de salida: 0 ok, 1 cobertura insuficiente, 2 error de uso o de lectura.
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
)

const minCoverage = 70.0

// checked son los paquetes con umbral; se comparan por el final de la ruta del directorio.
var checked = []string{"internal/game", "internal/engine"}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout))
}

// run separa la lógica de main para poder probarla: devuelve el código de salida en vez de llamar a os.Exit.
func run(args []string, out io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(out, "uso: covercheck <coverage.out>")
		return 2
	}
	f, err := os.Open(args[0])
	if err != nil {
		fmt.Fprintf(out, "covercheck: %v\n", err)
		return 2
	}
	defer f.Close()

	blocks, err := parse(f)
	if err != nil {
		fmt.Fprintf(out, "covercheck: %v\n", err)
		return 2
	}

	code := 0
	for _, pkg := range checked {
		total, covered := 0, 0
		for key, b := range blocks {
			if strings.HasSuffix(path.Dir(key.file), "/"+pkg) {
				total += b.stmts
				if b.hit {
					covered += b.stmts
				}
			}
		}
		if total == 0 {
			fmt.Fprintf(out, "%s: sin sentencias todavía (no se aplica el umbral)\n", pkg)
			continue
		}
		pct := float64(covered) * 100 / float64(total)
		status := "ok"
		if pct < minCoverage {
			status = fmt.Sprintf("FALLA: mínimo %.0f%%", minCoverage)
			code = 1
		}
		fmt.Fprintf(out, "%s: %.1f%% (%d/%d sentencias) %s\n", pkg, pct, covered, total, status)
	}
	return code
}

type blockKey struct{ file, pos string }

type block struct {
	stmts int
	hit   bool
}

// parse lee líneas "archivo.go:12.2,15.3 4 1" (bloque, sentencias, veces ejecutado).
// Un mismo bloque puede repetirse; se cuenta una vez y como cubierto si alguna línea lo ejecutó.
func parse(r io.Reader) (map[blockKey]block, error) {
	blocks := map[blockKey]block{}
	sc := bufio.NewScanner(r)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 3 {
			return nil, fmt.Errorf("línea %d mal formada: %q", lineNo, line)
		}
		colon := strings.LastIndex(fields[0], ":")
		stmts, err1 := strconv.Atoi(fields[1])
		count, err2 := strconv.Atoi(fields[2])
		if colon < 0 || err1 != nil || err2 != nil {
			return nil, fmt.Errorf("línea %d mal formada: %q", lineNo, line)
		}
		key := blockKey{file: fields[0][:colon], pos: fields[0][colon+1:]}
		b := blocks[key]
		b.stmts = stmts
		b.hit = b.hit || count > 0
		blocks[key] = b
	}
	return blocks, sc.Err()
}
