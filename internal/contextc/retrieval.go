package contextc

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/artumarinn/arg0s/internal/codegraph"
	"github.com/artumarinn/arg0s/internal/memory"
	memsqlite "github.com/artumarinn/arg0s/internal/memory/sqlite"
)

// NewQuery extrae Intent y entidades del prompt (sección 9, primer
// paso del flujo). El Intent es el prompt tal cual -- no hay
// resumen/reescritura de intención en Fase 3, eso es más elaborado que
// lo que esta fase pidió.
func NewQuery(prompt, project string) Query {
	return Query{
		Intent: prompt, Prompt: prompt, Project: project,
		Files: extractFiles(prompt), Symbols: extractSymbols(prompt),
	}
}

var fileToken = regexp.MustCompile(`[\w./-]+/[\w./-]+|[\w-]+\.[a-zA-Z]{1,4}`)
var symbolToken = regexp.MustCompile(`\b[A-Z][a-zA-Z0-9]*\b`)

// extractFiles busca tokens con pinta de path (contienen "/") o de
// archivo (nombre.ext) -- misma heurística barata que
// internal/router/heuristics.go, sin importar router para no acoplar
// contextc a él (dos paquetes distintos, mismo tipo de señal, cada uno
// dueño de su copia -- es una heurística de 1 línea, no una lógica de
// negocio para compartir).
func extractFiles(prompt string) []string {
	matches := fileToken.FindAllString(prompt, -1)
	return dedupe(matches)
}

// extractSymbols busca identificadores en PascalCase -- convención de
// Go/la mayoría de lenguajes tipados para tipos y funciones exportadas.
func extractSymbols(prompt string) []string {
	matches := symbolToken.FindAllString(prompt, -1)
	return dedupe(matches)
}

func dedupe(items []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, it := range items {
		if !seen[it] {
			seen[it] = true
			out = append(out, it)
		}
	}
	return out
}

// MemorySource envuelve memory.MemoryStore -- Fragment.Relevance
// reusa memsqlite.Relevance para no reinventar el ranking lexical acá
// (un solo dueño del score, igual que TaskProfile alias resolution).
type MemorySource struct {
	Store        memory.MemoryStore
	MinRelevance float64
}

func (s *MemorySource) Name() string { return "memory" }

func (s *MemorySource) Retrieve(ctx context.Context, q Query, budget int) ([]Fragment, error) {
	results, err := s.Store.Query(ctx, memory.MemoryQuery{
		Project: q.Project, Text: q.Intent, MinRelevance: s.MinRelevance,
	})
	if err != nil {
		return nil, fmt.Errorf("contextc: memory source: %w", err)
	}

	var frags []Fragment
	for _, m := range results {
		frags = append(frags, Fragment{
			Source: "memory", Ref: m.Title, Content: m.Content,
			Tokens: EstimateTokens(m.Content), Relevance: memsqlite.Relevance(q.Intent, m.Title, m.Content),
		})
	}
	return capByBudget(frags, budget*3), nil // recorte grueso anti-blowup; la enforcement fina (+compresión) la hace compiler.go
}

// CodeGraphSource localiza símbolos/archivos vía el grafo y LEE el
// contenido real acotado a su rango de líneas -- "el grafo localiza,
// no reemplaza" (sección 13): nunca se manda el grafo (nombres,
// metadata) en vez del código.
type CodeGraphSource struct {
	Graph *codegraph.Graph
}

func (s *CodeGraphSource) Name() string { return "codegraph" }

func (s *CodeGraphSource) Retrieve(ctx context.Context, q Query, budget int) ([]Fragment, error) {
	var frags []Fragment
	seen := map[string]bool{}

	addSymbol := func(sym codegraph.Symbol, relevance float64) {
		if seen[sym.ID] {
			return
		}
		seen[sym.ID] = true
		content, err := readLines(q.Project, sym.File, sym.LineStart, sym.LineEnd)
		if err != nil {
			return // archivo puntual no legible -- se lo salta, no aborta el retrieval
		}
		frags = append(frags, Fragment{
			Source: "codegraph", Ref: fmt.Sprintf("%s:%d-%d", sym.File, sym.LineStart, sym.LineEnd),
			Content: content, Tokens: EstimateTokens(content), Relevance: relevance,
		})
	}

	for _, name := range q.Symbols {
		results, err := s.Graph.FindSymbol(ctx, q.Project, name)
		if err != nil {
			return nil, fmt.Errorf("contextc: codegraph source: %w", err)
		}
		for _, sym := range results {
			addSymbol(sym, 1.0) // match exacto de nombre -- máxima relevancia
		}
	}

	// ponytail: RelatedFiles matchea por NOMBRE de símbolo (sección 13),
	// no por path de archivo -- un hint tipo "internal/router" (un
	// directorio, sin símbolo homónimo) no va a matchear nada acá y esa
	// entidad simplemente no aporta fragmentos. Suficiente para el caso
	// típico (el símbolo mencionado SÍ matchea en el loop de arriba);
	// una query real "archivos cuyo path contiene X" se agrega cuando
	// haga falta, sin inventarla ahora.
	for _, hint := range q.Files {
		files, err := s.Graph.RelatedFiles(ctx, q.Project, hint, 5)
		if err != nil {
			return nil, fmt.Errorf("contextc: codegraph source: %w", err)
		}
		for _, f := range files {
			syms, err := s.Graph.SymbolsInFile(ctx, q.Project, f.Path)
			if err != nil {
				return nil, fmt.Errorf("contextc: codegraph source: %w", err)
			}
			for _, sym := range syms {
				addSymbol(sym, 0.6) // vino de un hint de archivo, no de un match exacto de símbolo
			}
		}
	}

	sort.Slice(frags, func(i, j int) bool { return frags[i].Relevance > frags[j].Relevance })
	return capByBudget(frags, budget*3), nil // recorte grueso anti-blowup; la enforcement fina (+compresión) la hace compiler.go
}

func readLines(root, relFile string, start, end int) (string, error) {
	data, err := os.ReadFile(root + string(os.PathSeparator) + relFile)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	if start < 1 {
		start = 1
	}
	if end > len(lines) {
		end = len(lines)
	}
	if start > end {
		return "", fmt.Errorf("contextc: rango de líneas inválido %d-%d en %s", start, end, relFile)
	}
	return strings.Join(lines[start-1:end], "\n"), nil
}

// FileSource lee archivos EXPLÍCITAMENTE mencionados en el prompt con
// extensión (ej "internal/core/task.go") completos -- solo cuando el
// grafo no pudo acotar un rango (si pudo, ya lo cubrió
// CodeGraphSource con menos tokens). Es el fallback de "Files" del
// diagrama de flujo de sección 9, no el camino preferido.
type FileSource struct{}

func (s *FileSource) Name() string { return "file" }

func (s *FileSource) Retrieve(ctx context.Context, q Query, budget int) ([]Fragment, error) {
	var frags []Fragment
	for _, f := range q.Files {
		if !strings.Contains(f, ".") {
			continue // sin extensión -- probablemente un directorio/paquete, no un archivo
		}
		data, err := os.ReadFile(q.Project + string(os.PathSeparator) + f)
		if err != nil {
			continue
		}
		content := string(data)
		frags = append(frags, Fragment{Source: "file", Ref: f, Content: content, Tokens: EstimateTokens(content), Relevance: 0.5})
	}
	return capByBudget(frags, budget*3), nil // recorte grueso anti-blowup; la enforcement fina (+compresión) la hace compiler.go
}

// SkillsSource es el hook explícito de sección 9 -- Skills real es
// Fase 5 (registry, index-first, sección 14). Devuelve siempre vacío;
// existe para que el budget_split.skills y el Manifest ya tengan un
// lugar reservado sin inventar un sistema de skills a medias.
type SkillsSource struct{}

func (s *SkillsSource) Name() string { return "skill" }

func (s *SkillsSource) Retrieve(ctx context.Context, q Query, budget int) ([]Fragment, error) {
	return nil, nil
}

// capByBudget devuelve el prefijo de frags (YA ordenados por
// relevancia desc por el caller) que entra en budget tokens -- corta
// en el primero que no entra, no arma un bin-packing alrededor.
func capByBudget(frags []Fragment, budget int) []Fragment {
	var out []Fragment
	total := 0
	for _, f := range frags {
		if total+f.Tokens > budget {
			break
		}
		out = append(out, f)
		total += f.Tokens
	}
	return out
}
