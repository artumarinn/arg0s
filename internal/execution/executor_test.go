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

// newTestExecutor registra p con un catálogo de un solo modelo "m1"
// apuntando a él.
func newTestExecutor(t *testing.T, p *mock.Provider, policy ProviderPolicy) *Executor {
	t.Helper()
	registry := providers.NewRegistry()
	registry.Register(p)

	cat := catalog{
		"m1": {ID: "m1", Provider: p.Name(), CostInputPer1M: 1.0, CostOutputPer1M: 2.0},
	}
	policies := map[string]ProviderPolicy{p.Name(): policy}
	return New(registry, cat, policies, nil)
}

func req(prompt string) core.Request {
	return core.Request{ModelID: "m1", Prompt: prompt, Role: core.RoleGenerator}
}

func TestExecute_RetryExhaustsAttempts(t *testing.T) {
	p := mock.New(mock.Config{Name: "mock", FailAlways: true})
	e := newTestExecutor(t, p, ProviderPolicy{MaxRetries: 2})

	_, run, err := e.Execute(context.Background(), req("hola"))

	require.Error(t, err)
	require.Equal(t, 3, run.Attempts) // 1 + 2 retries
	require.Equal(t, 3, p.CallCount())
}

func TestExecute_RetrySucceedsOnThirdAttempt(t *testing.T) {
	p := mock.New(mock.Config{Name: "mock", FailFirstN: 2, Response: "listo"})
	e := newTestExecutor(t, p, ProviderPolicy{MaxRetries: 3})

	resp, run, err := e.Execute(context.Background(), req("hola"))

	require.NoError(t, err)
	require.Equal(t, "listo", resp.Content)
	require.Equal(t, 3, run.Attempts)
	require.True(t, run.Usage.Estimated)
	require.Positive(t, run.Usage.InputTokens)
}

func TestExecute_NoRetryOnFatalError(t *testing.T) {
	fatal := errors.New("auth failed: invalid api key")
	p := mock.New(mock.Config{Name: "mock", FailAlways: true, FailErr: fatal})
	e := newTestExecutor(t, p, ProviderPolicy{MaxRetries: 5})

	_, run, err := e.Execute(context.Background(), req("hola"))

	require.Error(t, err)
	require.Equal(t, 1, run.Attempts)
	require.Equal(t, 1, p.CallCount())
}

func TestExecute_ProviderTimeoutIsRetried(t *testing.T) {
	// policy.Timeout por-request (no el ctx del caller): el provider
	// "no respondió a tiempo" es RETRYABLE. El mock tiene Latency fija
	// mayor al Timeout en TODOS los intentos, así que termina agotando
	// el retry -- el punto del test es que sí reintentó (Attempts>1),
	// no que el timeout eventualmente rinda una respuesta OK.
	p := mock.New(mock.Config{Name: "mock", Latency: 200 * time.Millisecond})
	e := newTestExecutor(t, p, ProviderPolicy{MaxRetries: 1, Timeout: 20 * time.Millisecond})

	_, run, err := e.Execute(context.Background(), req("hola"))

	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, 2, run.Attempts, "timeout de provider retryable: debe haber reintentado")
}

func TestExecute_CallerDeadlineNeverRetries(t *testing.T) {
	// Acá el que vence es el ctx del CALLER (deadline corto), no el
	// timeout por-request de la policy (más largo o ausente). Debe
	// cortar en el primer intento, sin reintentar, aunque
	// DeadlineExceeded sea retryable en general.
	p := mock.New(mock.Config{Name: "mock", HangUntilCancel: true})
	e := newTestExecutor(t, p, ProviderPolicy{MaxRetries: 5}) // sin Timeout propio

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, run, err := e.Execute(ctx, req("hola"))
	elapsed := time.Since(start)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, 1, run.Attempts, "ctx.Err() del caller nunca dispara retry")
	require.Less(t, elapsed, 200*time.Millisecond)
}

func TestExecute_BackoffNeverExceedsCallerDeadline(t *testing.T) {
	// Backoff exponencial normal necesitaría varias esperas para 5
	// reintentos: con un deadline de 60ms no le alcanza. Debe cortar en
	// vez de dormir de más.
	p := mock.New(mock.Config{Name: "mock", FailAlways: true})
	e := newTestExecutor(t, p, ProviderPolicy{MaxRetries: 5, RetryBackoff: "linear"})

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, run, err := e.Execute(ctx, req("hola"))
	elapsed := time.Since(start)

	require.Error(t, err)
	require.Less(t, run.Attempts, 6, "no debe llegar a agotar los 6 intentos posibles dentro del deadline")
	require.Less(t, elapsed, 150*time.Millisecond, "no debe dormir más allá del deadline del caller")
}

func TestExecute_RetryAfterLongerThanDeadlineCutsInsteadOfSleeping(t *testing.T) {
	p := mock.New(mock.Config{Name: "mock", RateLimited: true, RetryAfter: 5 * time.Second})
	e := newTestExecutor(t, p, ProviderPolicy{MaxRetries: 3})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, run, err := e.Execute(ctx, req("hola"))
	elapsed := time.Since(start)

	require.Error(t, err)
	require.Equal(t, 1, run.Attempts, "el Retry-After de 5s excede el deadline de 30ms: no debe dormir, debe cortar")
	require.Less(t, elapsed, 200*time.Millisecond)
}

func TestExecute_CancellationPropagates(t *testing.T) {
	p := mock.New(mock.Config{Name: "mock", HangUntilCancel: true})
	e := newTestExecutor(t, p, ProviderPolicy{MaxRetries: 5})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, run, err := e.Execute(ctx, req("hola"))
	elapsed := time.Since(start)

	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, run.Attempts, "cancelar la task no debe disparar reintentos")
	require.Less(t, elapsed, 200*time.Millisecond)
}

func TestExecute_RateLimitRespectsRetryAfter(t *testing.T) {
	p := mock.New(mock.Config{Name: "mock", RateLimited: true, RetryAfter: 30 * time.Millisecond})
	e := newTestExecutor(t, p, ProviderPolicy{MaxRetries: 1})

	start := time.Now()
	_, run, err := e.Execute(context.Background(), req("hola"))
	elapsed := time.Since(start)

	require.Error(t, err)
	require.Equal(t, 2, run.Attempts)
	require.GreaterOrEqual(t, elapsed, 30*time.Millisecond, "debe esperar el Retry-After antes del 2do intento")
}

// TestExecute_FallbackHasOwnBudgetNotSharedWithPrimary documenta y
// prueba la decisión: el fallback tiene presupuesto PROPIO y acotado
// (1 intento, sin retry) — no hereda ni comparte los intentos que ya
// gastó el primario. Un provider primario caído no multiplica la
// espera intentando el fallback varias veces también.
func TestExecute_FallbackHasOwnBudgetNotSharedWithPrimary(t *testing.T) {
	primary := mock.New(mock.Config{Name: "primary", FailAlways: true})
	secondary := mock.New(mock.Config{Name: "secondary", FailFirstN: 1, Response: "no debería llegar acá"})

	registry := providers.NewRegistry()
	registry.Register(primary)
	registry.Register(secondary)

	cat := catalog{
		"m1": {ID: "m1", Provider: "primary"},
		"m2": {ID: "m2", Provider: "secondary"},
	}
	policies := map[string]ProviderPolicy{
		"primary":   {MaxRetries: 2}, // 3 intentos para el primario
		"secondary": {MaxRetries: 5}, // si el fallback compartiera retry, esto lo salvaría
	}
	e := New(registry, cat, policies, nil)

	fbReq := req("hola")
	fbReq.Fallback = "m2"
	_, run, err := e.Execute(context.Background(), fbReq)

	require.Error(t, err, "el fallback falla en su único intento (FailFirstN:1) y no tiene retry propio pese a MaxRetries:5")
	require.Equal(t, 3, primary.CallCount())
	require.Equal(t, 1, secondary.CallCount(), "el fallback se intenta UNA sola vez, ignora policy.MaxRetries del fallback")
	require.Equal(t, 4, run.Attempts)
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

	fbReq := req("hola")
	fbReq.Fallback = "m2"
	resp, run, err := e.Execute(context.Background(), fbReq)

	require.NoError(t, err)
	require.Equal(t, "desde fallback", resp.Content)
	require.Equal(t, "m2", run.ModelID)
	require.Equal(t, "m1", run.FellBackFrom)
	require.Equal(t, 1, secondary.CallCount())
	require.Equal(t, 2, primary.CallCount()) // 1 intento + 1 retry antes de rendirse
}

func TestExecute_MaxCallsCapsPrimaryPlusFallback(t *testing.T) {
	primary := mock.New(mock.Config{Name: "primary", FailAlways: true})
	secondary := mock.New(mock.Config{Name: "secondary", Response: "no debería llegar acá"})

	registry := providers.NewRegistry()
	registry.Register(primary)
	registry.Register(secondary)

	cat := catalog{
		"m1": {ID: "m1", Provider: "primary"},
		"m2": {ID: "m2", Provider: "secondary"},
	}
	policies := map[string]ProviderPolicy{
		"primary":   {MaxRetries: 5}, // sin el cap de MaxCalls, serían 6 intentos
		"secondary": {MaxRetries: 0},
	}
	e := New(registry, cat, policies, nil)

	fbReq := req("hola")
	fbReq.Fallback = "m2"
	fbReq.MaxCalls = 2 // limits.per_task_max_calls
	_, run, err := e.Execute(context.Background(), fbReq)

	require.Error(t, err)
	require.LessOrEqual(t, run.Attempts, 2, "MaxCalls debe acotar primario+fallback juntos")
	require.Equal(t, 2, primary.CallCount())
	require.Equal(t, 0, secondary.CallCount(), "el presupuesto ya se agotó en el primario, el fallback ni se intenta")
}

func TestExecute_AccountingOnSuccessAndFailure(t *testing.T) {
	okProvider := mock.New(mock.Config{Name: "ok", Response: "una respuesta cualquiera"})
	eOK := newTestExecutor(t, okProvider, ProviderPolicy{})
	_, runOK, err := eOK.Execute(context.Background(), req("prompt de prueba"))
	require.NoError(t, err)
	require.True(t, runOK.Usage.Estimated)
	require.Positive(t, runOK.Usage.InputTokens)
	require.Positive(t, runOK.Usage.OutputTokens)
	require.Positive(t, runOK.Usage.CostUSD)

	failProvider := mock.New(mock.Config{Name: "fail", FailAlways: true})
	eFail := newTestExecutor(t, failProvider, ProviderPolicy{})
	_, runFail, err := eFail.Execute(context.Background(), req("prompt de prueba"))
	require.Error(t, err)
	require.Positive(t, runFail.Usage.InputTokens, "el input ya se gastó aunque falle la llamada")
	require.Zero(t, runFail.Usage.OutputTokens)
}

func TestExecute_RedactsAPIKeyFromErrors(t *testing.T) {
	const fakeKey = "sk-ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	fatal := errors.New("auth failed for key " + fakeKey)
	p := mock.New(mock.Config{Name: "mock", FailAlways: true, FailErr: fatal})
	e := newTestExecutor(t, p, ProviderPolicy{})

	_, run, err := e.Execute(context.Background(), req("hola"))

	require.Error(t, err)
	require.NotContains(t, err.Error(), fakeKey, "la key no debe aparecer en el error devuelto por Execute")
	require.NotContains(t, run.Err.Error(), fakeKey, "la key no debe aparecer en ModelRun.Err")
	require.Contains(t, err.Error(), "sk-...")
}

func TestExecute_RateLimitSerializesConcurrency(t *testing.T) {
	var active, maxActive int32

	p := mock.New(mock.Config{Name: "mock", ResponseFn: func(r core.Request) string {
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
	e := newTestExecutor(t, p, ProviderPolicy{Concurrent: 1})

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _ = e.Execute(context.Background(), req("x"))
		}()
	}
	wg.Wait()

	require.Equal(t, int32(1), atomic.LoadInt32(&maxActive), "concurrent:1 debe serializar las 3 llamadas")
}

func TestStream_AccountsUsageOnSuccess(t *testing.T) {
	p := mock.New(mock.Config{Name: "mock", Response: "una respuesta bien larga para trocear en chunks"})
	e := newTestExecutor(t, p, ProviderPolicy{})

	ch, run, err := e.Stream(context.Background(), req("prompt de prueba"))
	require.NoError(t, err)

	var full string
	for c := range ch {
		full += c.Delta
	}

	require.Contains(t, full, "una respuesta")
	require.NoError(t, run.Err)
	require.Positive(t, run.Usage.InputTokens)
	require.Positive(t, run.Usage.OutputTokens)
	require.Positive(t, run.Usage.CostUSD)
}

func TestStream_InterruptedMidwayStillRecordsPartialUsage(t *testing.T) {
	// ChunkDelay le da ventana real al cancel(): sin esto, un stream de
	// contenido corto termina en microsegundos y la cancelación nunca
	// alcanza a interponerse (flaky sin esta pacing).
	p := mock.New(mock.Config{
		Name:       "mock",
		Response:   "contenido bastante largo para que alcance a mandar varios chunks antes de cortar",
		ChunkDelay: 30 * time.Millisecond,
	})
	e := newTestExecutor(t, p, ProviderPolicy{})

	ctx, cancel := context.WithCancel(context.Background())
	ch, run, err := e.Stream(ctx, req("prompt de prueba"))
	require.NoError(t, err)

	// Consumimos un solo chunk y cancelamos -- simula ctrl+c a mitad de
	// un streaming real.
	<-ch
	cancel()
	for range ch {
		// drenar hasta que el productor cierre el canal tras ver ctx.Done()
	}

	require.Error(t, run.Err)
	require.ErrorIs(t, run.Err, context.Canceled)
	require.Positive(t, run.Usage.InputTokens)
	require.Positive(t, run.Usage.OutputTokens, "el primer chunk ya generado cuenta como tokens consumidos")
}
