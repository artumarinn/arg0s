package router

import (
	"context"
	"fmt"
	"time"
)

// CostSource da acceso al gasto acumulado -- desacoplado de
// internal/storage (mismo patrón que execution.ModelCatalog) para que
// router no dependa del paquete de persistencia concreto.
type CostSource interface {
	CostSinceUSD(ctx context.Context, since time.Time) (float64, error)
}

// BudgetExceededError es el bloqueo "MÍNIMO" de sección 8 punto 7:
// solo per_task_cost_usd y daily_cost_usd contra lo acumulado. Mensaje
// claro, nunca un fallo silencioso ni un panic.
type BudgetExceededError struct {
	Limit string // "per_task" | "daily"
	Msg   string
}

func (e *BudgetExceededError) Error() string { return e.Msg }

// checkAndReserveBudget estima el costo de una tarea como el tope del
// tier candidato (tier.max_cost_usd) -- no hay estimación de tokens
// real todavía (Context Compiler es Fase 3), así que se usa el peor
// caso declarado en config.yaml.
//
// El chequeo y la reserva son atómicos bajo r.mu: dos llamadas
// concurrentes (dos goroutines de un mismo proceso pidiendo Decide al
// mismo tiempo) no pueden las dos ver "hay lugar" y pasar juntas por
// encima de daily_cost_usd -- la primera que entra reserva su estimado
// en memoria (reservedUSD) antes de soltar el lock, así que la segunda
// ve ese gasto ya comprometido aunque todavía no se haya persistido
// ningún run. La reserva es SOLO in-memory y vive mientras dure el
// proceso -- no protege contra dos procesos `arg0s run` separados
// corriendo a la vez; eso necesita un lock real (daemon, Fase 4).
func (r *Router) checkAndReserveBudget(ctx context.Context, estimatedUSD float64) error {
	if r.limits.PerTaskCostUSD > 0 && estimatedUSD > r.limits.PerTaskCostUSD {
		return &BudgetExceededError{
			Limit: "per_task",
			Msg:   fmt.Sprintf("bloqueado: la tarea puede costar hasta $%.4f, por encima de limits.per_task_cost_usd ($%.4f)", estimatedUSD, r.limits.PerTaskCostUSD),
		}
	}
	if r.limits.DailyCostUSD <= 0 || r.costs == nil {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	today := startOfDay(time.Now())
	if !r.reservedDay.Equal(today) {
		r.reservedDay = today
		r.reservedUSD = 0
	}

	usedToday, err := r.costs.CostSinceUSD(ctx, today)
	if err != nil {
		return fmt.Errorf("router: no se pudo leer el gasto diario: %w", err)
	}
	if usedToday+r.reservedUSD+estimatedUSD > r.limits.DailyCostUSD {
		return &BudgetExceededError{
			Limit: "daily",
			Msg: fmt.Sprintf("bloqueado: gasto de hoy $%.4f (+ $%.4f ya reservado por otra corrida) + hasta $%.4f de esta tarea supera limits.daily_cost_usd ($%.4f)",
				usedToday, r.reservedUSD, estimatedUSD, r.limits.DailyCostUSD),
		}
	}

	r.reservedUSD += estimatedUSD
	return nil
}

func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}
