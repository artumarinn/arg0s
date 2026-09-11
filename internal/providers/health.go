package providers

import (
	"context"
	"fmt"
	"time"

	"github.com/artumarinn/arg0s/internal/core"
)

// Health corre un request mínimo (Models) contra cada provider
// registrado — checks 6 y 7 de la sección 6.7 (conectividad real +
// modelos locales cargados, en el caso de Ollama). Solo-lectura: nunca
// escribe estado de Arg0s (regla dura #11); la llamada de red en sí es
// justamente lo que el check pide verificar.
func (r *Registry) Health(ctx context.Context, timeout time.Duration) []core.Check {
	var checks []core.Check
	for _, name := range r.Names() {
		p, ok := r.Get(name)
		if !ok {
			continue
		}

		callCtx, cancel := context.WithTimeout(ctx, timeout)
		start := time.Now()
		models, err := p.Models(callCtx)
		latency := time.Since(start)
		cancel()

		if err != nil {
			checks = append(checks, core.Check{
				Group: "Providers", Name: name + " connectivity", Status: core.CheckFail,
				Detail: core.RedactSecrets(err.Error()),
			})
			continue
		}

		detail := fmt.Sprintf("reachable %dms", latency.Milliseconds())
		if name == "ollama" {
			detail += fmt.Sprintf(" (%d models loaded)", len(models))
		}
		checks = append(checks, core.Check{Group: "Providers", Name: name + " connectivity", Status: core.CheckPass, Detail: detail})
	}
	return checks
}
