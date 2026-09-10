package core

import "errors"

// Errores tipados para clasificación en el executor (ver docs/ARG0S.md sección 22).
var (
	ErrRetryable   = errors.New("retryable")
	ErrRateLimited = errors.New("rate limited")
	ErrBudget      = errors.New("budget exceeded")
	ErrPolicy      = errors.New("policy denied")
	ErrUnavailable = errors.New("provider unavailable")
)
