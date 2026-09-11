package core

// Request es una llamada a un modelo concreto — la granularidad en la
// que trabaja el Executor. Distinto de Task: una Task puede disparar
// varios Request (fusion, judgment); un Request es una sola llamada.
//
// Role, Fallback y MaxCalls viven acá (no como parámetros posicionales
// de Execute) porque van a seguir creciendo con cada fase — presupuesto
// de contexto, blindness de jueces, límites por task. Un parámetro más
// por fase no escala; el dato sí tiene que estar disponible para el
// executor.
type Request struct {
	ModelID     string
	Prompt      string
	Messages    []Message
	MaxTokens   int
	Temperature float64

	// Role es para trazabilidad (ModelRun.Role) — qué función del
	// sistema hizo esta llamada (generator, judge_a, fix_agent...).
	Role Role

	// Fallback es el ModelID a intentar UNA sola vez (sin retry propio)
	// si ModelID se agota sin éxito. Vacío = sin fallback.
	Fallback string

	// MaxCalls acota el total de llamadas de ESTE Execute — primario +
	// sus retries + el intento de fallback si lo hay. 0 = sin límite
	// propio (rige policy.MaxRetries). Mapea a limits.per_task_max_calls.
	MaxCalls int
}

// Response es la salida de un Request exitoso.
type Response struct {
	ModelID      string
	Content      string
	Usage        Usage
	FinishReason string
}

// Chunk es un fragmento de una respuesta en streaming. El chunk final
// trae Done=true y, si el provider lo reporta, Usage.
type Chunk struct {
	Delta string
	Usage *Usage
	Done  bool
	Err   error
}
