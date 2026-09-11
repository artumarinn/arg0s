package execution

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/artumarinn/arg0s/internal/core"
	"github.com/artumarinn/arg0s/internal/providers"
	"github.com/artumarinn/arg0s/internal/providers/mock"
)

// catalog es un ModelCatalog mínimo para tests: model ID -> core.Model.
type catalog map[string]core.Model

func (c catalog) Resolve(id string) (core.Model, bool) {
	m, ok := c[id]
	return m, ok
}

// newTestExecutor registra p bajo el nombre de provider "mock" (o el
// que tenga p.Name()), con un catálogo de un solo modelo apuntando a él.
func newTestExecutor(t *testing.T, p *mock.Provider, policy ProviderPolicy) (*Executor, catalog) {
	t.Helper()
	registry := providers.NewRegistry()
	registry.Register(p)

	cat := catalog{
		"m1": {ID: "m1", Provider: p.Name(), CostInputPer1M: 1.0, CostOutputPer1M: 2.0},
	}

	policies := map[string]ProviderPolicy{p.Name(): policy}
	return New(registry, cat, policies, nil), cat
}

func TestExecute_RetryExhaustsAttempts(t *testing.T) {
	p := mock.New(mock.Config{Name: "mock", FailAlways: true})
	e, _ := newTestExecutor(t, p, ProviderPolicy{MaxRetries: 2})

	_, run, err := e.Execute(context.Background(), core.Request{ModelID: "m1", Prompt: "hola"}, core.RoleGenerator, "")

	require.Error(t, err)
	require.Equal(t, 3, run.Attempts) // 1 + 2 retries
	require.Equal(t, 3, p.CallCount())
}

func TestExecute_RetrySucceedsOnThirdAttempt(t *testing.T) {
	p := mock.New(mock.Config{Name: "mock", FailFirstN: 2, Response: "listo"})
	e, _ := newTestExecutor(t, p, ProviderPolicy{MaxRetries: 3})

	resp, run, err := e.Execute(context.Background(), core.Request{ModelID: "m1", Prompt: "hola"}, core.RoleGenerator, "")

	require.NoError(t, err)
	require.Equal(t, "listo", resp.Content)
	require.Equal(t, 3, run.Attempts)
	require.True(t, run.Usage.Estimated)
	require.Positive(t, run.Usage.InputTokens)
}

func TestExecute_NoRetryOnFatalError(t *testing.T) {
	fatal := errors.New("auth failed: invalid api key")
	p := mock.New(mock.Config{Name: "mock", FailAlways: true, FailErr: fatal})
	e, _ := newTestExecutor(t, p, ProviderPolicy{MaxRetries: 5})

	_, run, err := e.Execute(context.Background(), core.Request{ModelID: "m1", Prompt: "hola"}, core.RoleGenerator, "")

	require.Error(t, err)
	require.Equal(t, 1, run.Attempts)
	require.Equal(t, 1, p.CallCount())
}

func TestExecute_TimeoutCancelsInFlightCall(t *testing.T) {
	p := mock.New(mock.Config{Name: "mock", Latency: 500 * time.Millisecond})
	e, _ := newTestExecutor(t, p, ProviderPolicy{MaxRetries: 0, Timeout: 20 * time.Millisecond})

	start := time.Now()
	_, run, err := e.Execute(context.Background(), core.Request{ModelID: "m1", Prompt: "hola"}, core.RoleGenerator, "")
	elapsed := time.Since(start)

	require.Error(t, err)
	require.Equal(t, 1, run.Attempts)
	require.Less(t, elapsed, 200*time.Millisecond, "el timeout de policy debe cortar mucho antes de los 500ms de latencia")
}

func TestExecute_CancellationPropagates(t *testing.T) {
	p := mock.New(mock.Config{Name: "mock", HangUntilCancel: true})
	e, _ := newTestExecutor(t, p, ProviderPolicy{MaxRetries: 5})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, run, err := e.Execute(ctx, core.Request{ModelID: "m1", Prompt: "hola"}, core.RoleGenerator, "")
	elapsed := time.Since(start)

	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, run.Attempts, "cancelar la task no debe disparar reintentos")
	require.Less(t, elapsed, 200*time.Millisecond)
}

func TestExecute_RateLimitRespectsRetryAfter(t *testing.T) {
	p := mock.New(mock.Config{Name: "mock", RateLimited: true, RetryAfter: 30 * time.Millisecond})
	e, _ := newTestExecutor(t, p, ProviderPolicy{MaxRetries: 1})

	start := time.Now()
	_, run, err := e.Execute(context.Background(), core.Request{ModelID: "m1", Prompt: "hola"}, core.RoleGenerator, "")
	elapsed := time.Since(start)

	require.Error(t, err)
	require.Equal(t, 2, run.Attempts)
	require.GreaterOrEqual(t, elapsed, 30*time.Millisecond, "debe esperar el Retry-After antes del 2do intento")
}

func TestExecute_FallbackUsedAndRecorded(t *testing.T) {
	primary := mock.New(mock.Config{Name: "primary", FailAlways: true})
	secondary := mock.New(mock.Config{Name: "secondary", Response: "desde fallback"})

	registry := providers.NewRegistry()
	registry.Register(primary)
	registry.Register(secondary)

	cat := catalog{
		"m1": {ID: "m1", Provider: "primary"},
		"m2": {ID: "m2", Provider: "secondary"},
	}
	policies := map[string]ProviderPolicy{
		"primary":   {MaxRetries: 1},
		"secondary": {MaxRetries: 0},
	}
	e := New(registry, cat, policies, nil)

	resp, run, err := e.Execute(context.Background(), core.Request{ModelID: "m1", Prompt: "hola"}, core.RoleGenerator, "m2")

	require.NoError(t, err)
	require.Equal(t, "desde fallback", resp.Content)
	require.Equal(t, "m2", run.ModelID)
	require.Equal(t, "m1", run.FellBackFrom)
	require.Equal(t, 1, secondary.CallCount())
	require.Equal(t, 2, primary.CallCount()) // 1 intento + 1 retry antes de rendirse
}

func TestExecute_AccountingOnSuccessAndFailure(t *testing.T) {
	okProvider := mock.New(mock.Config{Name: "ok", Response: "una respuesta cualquiera"})
	eOK, _ := newTestExecutor(t, okProvider, ProviderPolicy{})
	_, runOK, err := eOK.Execute(context.Background(), core.Request{ModelID: "m1", Prompt: "prompt de prueba"}, core.RoleGenerator, "")
	require.NoError(t, err)
	require.True(t, runOK.Usage.Estimated)
	require.Positive(t, runOK.Usage.InputTokens)
	require.Positive(t, runOK.Usage.OutputTokens)
	require.Positive(t, runOK.Usage.CostUSD)

	failProvider := mock.New(mock.Config{Name: "fail", FailAlways: true})
	eFail, _ := newTestExecutor(t, failProvider, ProviderPolicy{})
	_, runFail, err := eFail.Execute(context.Background(), core.Request{ModelID: "m1", Prompt: "prompt de prueba"}, core.RoleGenerator, "")
	require.Error(t, err)
	require.Positive(t, runFail.Usage.InputTokens, "el input ya se gastó aunque falle la llamada")
	require.Zero(t, runFail.Usage.OutputTokens)
}

func TestExecute_RedactsAPIKeyFromErrors(t *testing.T) {
	const fakeKey = "sk-ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	fatal := errors.New("auth failed for key " + fakeKey)
	p := mock.New(mock.Config{Name: "mock", FailAlways: true, FailErr: fatal})
	e, _ := newTestExecutor(t, p, ProviderPolicy{})

	_, run, err := e.Execute(context.Background(), core.Request{ModelID: "m1", Prompt: "hola"}, core.RoleGenerator, "")

	require.Error(t, err)
	require.NotContains(t, err.Error(), fakeKey, "la key no debe aparecer en el error devuelto por Execute")
	require.NotContains(t, run.Err.Error(), fakeKey, "la key no debe aparecer en ModelRun.Err")
	require.Contains(t, err.Error(), "sk-...")
}

func TestExecute_RateLimitSerializesConcurrency(t *testing.T) {
	var active, maxActive int32

	p := mock.New(mock.Config{Name: "mock", ResponseFn: func(req core.Request) string {
		n := atomic.AddInt32(&active, 1)
		for {
			old := atomic.LoadInt32(&maxActive)
			if n <= old || atomic.CompareAndSwapInt32(&maxActive, old, n) {
				break
			}
		}
		time.Sleep(30 * time.Millisecond) // ventana en la que se puede pisar con otra goroutine
		atomic.AddInt32(&active, -1)
		return "ok"
	}})
	e, _ := newTestExecutor(t, p, ProviderPolicy{Concurrent: 1})

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _ = e.Execute(context.Background(), core.Request{ModelID: "m1", Prompt: "x"}, core.RoleGenerator, "")
		}()
	}
	wg.Wait()

	require.Equal(t, int32(1), atomic.LoadInt32(&maxActive), "concurrent:1 debe serializar las 3 llamadas")
}
