package core

// Request es una llamada a un modelo concreto — la granularidad en la
// que trabaja el Executor. Distinto de Task: una Task puede disparar
// varios Request (fusion, judgment); un Request es una sola llamada.
type Request struct {
	ModelID     string
	Prompt      string
	Messages    []Message
	MaxTokens   int
	Temperature float64
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
