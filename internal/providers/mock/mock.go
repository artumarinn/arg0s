// Package mock es un Provider determinista para tests — retry, timeout,
// cancelación, fallback y accounting del executor se prueban contra
// esto, nunca contra red real.
package mock

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/artumarinn/arg0s/internal/core"
)

// Config controla el comportamiento del mock. Todos los campos son
// opcionales; el zero value es "responde OK con contenido vacío".
type Config struct {
	Name string // default "mock"

	Response   string                        // contenido fijo de la respuesta
	ResponseFn func(req core.Request) string // si está seteado, gana sobre Response

	Latency         time.Duration // delay artificial antes de responder, cancelable por ctx
	HangUntilCancel bool          // no responde nunca; solo retorna al cancelarse el ctx

	FailFirstN int   // las primeras N llamadas fallan con FailErr, la N+1 responde OK
	FailAlways bool  // ignora FailFirstN, siempre falla con FailErr
	FailErr    error // default: error que envuelve core.ErrRetryable

	RateLimited bool          // responde siempre con *core.RateLimitError
	RetryAfter  time.Duration // Retry-After del RateLimitError

	Usage  core.Usage   // usage a reportar en éxito; zero value = "el provider no reportó tokens"
	Models []core.Model // catálogo que devuelve Models(); default: un modelo sintético
}

type Provider struct {
	cfg Config

	mu    sync.Mutex
	calls int
}

func New(cfg Config) *Provider {
	if cfg.Name == "" {
		cfg.Name = "mock"
	}
	if cfg.FailErr == nil {
		cfg.FailErr = fmt.Errorf("mock: llamada fallida: %w", core.ErrRetryable)
	}
	return &Provider{cfg: cfg}
}

func (p *Provider) Name() string { return p.cfg.Name }

func (p *Provider) Models(ctx context.Context) ([]core.Model, error) {
	if len(p.cfg.Models) > 0 {
		return p.cfg.Models, nil
	}
	return []core.Model{{ID: "mock-model", Provider: p.cfg.Name, Tier: "simple"}}, nil
}

// CallCount devuelve cuántas veces se llamó Complete — útil para
// verificar en tests que retry/fallback llamaron la cantidad esperada.
func (p *Provider) CallCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func (p *Provider) nextCall() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	return p.calls
}

func (p *Provider) Complete(ctx context.Context, req core.Request) (core.Response, error) {
	if p.cfg.HangUntilCancel {
		<-ctx.Done()
		return core.Response{}, ctx.Err()
	}

	if p.cfg.Latency > 0 {
		select {
		case <-time.After(p.cfg.Latency):
		case <-ctx.Done():
			return core.Response{}, ctx.Err()
		}
	}

	call := p.nextCall()

	if p.cfg.RateLimited {
		return core.Response{}, &core.RateLimitError{RetryAfter: p.cfg.RetryAfter}
	}
	if p.cfg.FailAlways || call <= p.cfg.FailFirstN {
		return core.Response{}, p.cfg.FailErr
	}

	content := p.cfg.Response
	if p.cfg.ResponseFn != nil {
		content = p.cfg.ResponseFn(req)
	}

	return core.Response{
		ModelID: req.ModelID,
		Content: content,
		Usage:   p.cfg.Usage, // zero value = "el provider no reportó tokens"; el executor estima
	}, nil
}

// Stream parte Complete en unos pocos Chunk. No es un streaming real
// (el mock no tiene nada que streamear incrementalmente) — alcanza para
// probar que el consumidor procesa chunks y respeta cancelación.
func (p *Provider) Stream(ctx context.Context, req core.Request) (<-chan core.Chunk, error) {
	resp, err := p.Complete(ctx, req)
	if err != nil {
		return nil, err
	}

	ch := make(chan core.Chunk)
	go func() {
		defer close(ch)
		const pieces = 4
		content := resp.Content
		step := (len(content) + pieces - 1) / pieces
		if step == 0 {
			step = 1
		}
		for i := 0; i < len(content); i += step {
			end := min(i+step, len(content))
			select {
			case ch <- core.Chunk{Delta: content[i:end]}:
			case <-ctx.Done():
				return
			}
		}
		usage := resp.Usage
		select {
		case ch <- core.Chunk{Done: true, Usage: &usage}:
		case <-ctx.Done():
		}
	}()
	return ch, nil
}
