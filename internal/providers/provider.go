// Package providers define la interfaz Provider (endpoint de inferencia)
// y su registry. No importa internal/config — quien construye un
// Provider concreto recibe los parámetros que necesita como structs
// simples definidos en su propio paquete (ver ADR: providers/config
// desacoplados).
package providers

import (
	"context"
	"sort"
	"sync"

	"github.com/artumarinn/arg0s/internal/core"
)

// Provider es un endpoint de inferencia (Gemini API, Ollama, etc).
// Distinto de Runtime (agente con loop propio) — ver docs/ARG0S.md
// sección 4.2.
type Provider interface {
	Name() string
	Models(ctx context.Context) ([]core.Model, error)
	Complete(ctx context.Context, req core.Request) (core.Response, error)
	Stream(ctx context.Context, req core.Request) (<-chan core.Chunk, error)
}

// Registry mapea nombre de provider -> implementación. Poblado en el
// arranque del cliente/daemon según qué providers estén enabled.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

func NewRegistry() *Registry {
	return &Registry{providers: map[string]Provider{}}
}

func (r *Registry) Register(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.Name()] = p
}

func (r *Registry) Get(name string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	return p, ok
}

// Names devuelve los providers registrados, ordenados.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.providers))
	for n := range r.providers {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
