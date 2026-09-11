package execution

import (
	"context"
	"time"
)

// withTimeout aplica d como deadline sobre ctx. d<=0 significa "sin
// timeout propio" — se hereda el del contexto padre (típicamente el de
// la Task completa).
func withTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, d)
}
