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
)

// ModelRun traza una llamada individual a un modelo dentro de un Result.
type ModelRun struct {
	ModelID  string
	Role     Role
	Usage    Usage
	Latency  time.Duration
	Attempts int
	Err      error
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
}
