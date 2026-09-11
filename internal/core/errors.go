package core

import (
	"errors"
	"fmt"
	"time"
)

// Errores tipados para clasificación en el executor (ver docs/ARG0S.md sección 22).
var (
	ErrRetryable   = errors.New("retryable")
	ErrRateLimited = errors.New("rate limited")
	ErrBudget      = errors.New("budget exceeded")
	ErrPolicy      = errors.New("policy denied")
	ErrUnavailable = errors.New("provider unavailable")
)

// RateLimitError es un 429 con el Retry-After que reportó el provider
// (si vino). El executor lo respeta en vez de calcular su propio backoff.
type RateLimitError struct {
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("rate limited, retry after %s", e.RetryAfter)
}

// Unwrap deja que errors.Is(err, ErrRateLimited) siga funcionando.
func (e *RateLimitError) Unwrap() error { return ErrRateLimited }
