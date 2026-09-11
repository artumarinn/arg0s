package execution

import (
	"context"
	"errors"
	"math/rand"
	"time"

	"github.com/artumarinn/arg0s/internal/core"
	"github.com/artumarinn/arg0s/internal/providers"
	"github.com/artumarinn/arg0s/internal/telemetry"
)

// retryBase es la base del backoff exponencial. Chico a propósito para
// que los tests de retry no dependan de sleeps largos.
const retryBase = 20 * time.Millisecond

// executeWithRetry reintenta Complete hasta maxAttempts veces, solo
// sobre errores retryables (core.ErrRetryable, *core.RateLimitError,
// timeout del provider). Nunca reintenta sobre error de contenido/auth.
//
// Regla dura: el ctx del caller (deadline o Cancel) NUNCA dispara
// retry, aunque el error en sí sería retryable — se chequea ctx.Err()
// antes de cada intento, no solo después de que falle uno. El backoff
// tampoco duerme más allá del deadline del ctx: si el próximo sleep lo
// excedería, corta ahí en vez de dormir de más.
func (e *Executor) executeWithRetry(ctx context.Context, p providers.Provider, req core.Request, policy ProviderPolicy, maxAttempts int) (core.Response, int, error) {
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	var lastErr error
	made := 0
	for made < maxAttempts {
		if err := ctx.Err(); err != nil {
			return core.Response{}, made, err
		}

		callCtx, cancel := withTimeout(ctx, policy.Timeout)
		resp, err := p.Complete(callCtx, req)
		cancel()
		made++

		if err == nil {
			return resp, made, nil
		}
		lastErr = err

		if ctx.Err() != nil {
			return core.Response{}, made, ctx.Err()
		}
		if !isRetryable(err) {
			return core.Response{}, made, err
		}
		if made == maxAttempts {
			break
		}

		wait := backoffDelay(policy.RetryBackoff, made)
		var rlErr *core.RateLimitError
		if errors.As(err, &rlErr) && rlErr.RetryAfter > 0 {
			wait = rlErr.RetryAfter
		}
		if deadline, ok := ctx.Deadline(); ok && wait > time.Until(deadline) {
			// Dormir superaría el deadline del caller — fallar ya en
			// vez de dormir de más y devolver tarde de cualquier forma.
			return core.Response{}, made, lastErr
		}

		e.emit(ctx, telemetry.EventModelRetried, req.ModelID, req.Role, err)

		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return core.Response{}, made, ctx.Err()
		}
	}
	return core.Response{}, made, lastErr
}

// isRetryable: 5xx (core.ErrRetryable), timeout del provider y rate
// limit — nunca error de contenido o auth (sección 7). El timeout acá
// es el de policy.Timeout por request (context.DeadlineExceeded del
// callCtx derivado) — no confundir con el ctx del caller, que se
// chequea aparte y siempre corta sin retry.
func isRetryable(err error) bool {
	var rlErr *core.RateLimitError
	if errors.As(err, &rlErr) {
		return true
	}
	return errors.Is(err, core.ErrRetryable) || errors.Is(err, context.DeadlineExceeded)
}

func backoffDelay(mode string, attempt int) time.Duration {
	var d time.Duration
	switch mode {
	case "linear":
		d = retryBase * time.Duration(attempt)
	case "none":
		d = 0
	default: // exponential
		d = retryBase * time.Duration(uint(1)<<uint(attempt-1))
	}
	if d == 0 {
		return 0
	}
	return d + time.Duration(rand.Int63n(int64(retryBase)))
}
