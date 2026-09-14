package core

import "time"

type Role string

const (
	RoleGenerator   Role = "generator"
	RoleJudgeA      Role = "judge_a"
	RoleJudgeB      Role = "judge_b"
	RoleSynthesizer Role = "synthesizer"
	RoleFix         Role = "fix"
	RoleClassifier  Role = "classifier"
	RoleSummarizer  Role = "summarizer"
)

// ModelRun traza una llamada individual a un modelo dentro de un Result.
type ModelRun struct {
	ModelID  string
	Role     Role
	Usage    Usage
	Latency  time.Duration
	Attempts int
	Err      error
	// FellBackFrom es el ModelID original si esta corrida terminó
	// usando el fallback del rol (sección 7: "se intenta UNA sola vez
	// y lo registra como tal en el ModelRun"). Vacío si no hubo fallback.
	FellBackFrom string
}

// Result es la salida de ejecutar una Task bajo una Strategy.
type Result struct {
	TaskID    TaskID
	Content   string
	Artifacts []Artifact
	Usage     Usage
	Strategy  StrategyName
	ModelRuns []ModelRun
	StartedAt time.Time
	EndedAt   time.Time
	Err       error
	// Metadata es extra por-strategy que no amerita su propio campo acá
	// -- ej fusion.Result.Decision/Divergence, para que `arg0s run`
	// pueda mostrar "no escaló" / "sintetizó" sin que core importe fusion.
	Metadata map[string]any
}
