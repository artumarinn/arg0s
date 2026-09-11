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

// executeWithRetry reintenta Complete solo sobre errores retryables
// (core.ErrRetryable o *core.RateLimitError). Nunca reintenta sobre
// error de contenido/auth, ni si el ctx de la Task ya se canceló.
func (e *Executor) executeWithRetry(ctx context.Context, p providers.Provider, req core.Request, policy ProviderPolicy) (core.Response, int, error) {
	maxAttempts := policy.MaxRetries + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		callCtx, cancel := withTimeout(ctx, policy.Timeout)
		resp, err := p.Complete(callCtx, req)
		cancel()

		if err == nil {
			return resp, attempt, nil
		}
		lastErr = err

		if ctx.Err() != nil {
			// El contexto de la Task (no el de este request) ya se
			// canceló/venció — no tiene sentido reintentar.
			return core.Response{}, attempt, ctx.Err()
		}
		if !isRetryable(err) {
			return core.Response{}, attempt, err
		}
		if attempt == maxAttempts {
			break
		}

		wait := backoffDelay(policy.RetryBackoff, attempt)
		var rlErr *core.RateLimitError
		if errors.As(err, &rlErr) && rlErr.RetryAfter > 0 {
			wait = rlErr.RetryAfter
		}

		e.emit(ctx, telemetry.EventModelRetried, req.ModelID, "", err)

		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return core.Response{}, attempt, ctx.Err()
		}
	}
	return core.Response{}, maxAttempts, lastErr
}

// isRetryable: 5xx (core.ErrRetryable), timeout y rate limit — nunca
// error de contenido o auth (sección 7).
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
