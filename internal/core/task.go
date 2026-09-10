package core

import "time"

type TaskID string
type SessionID string

type Level string

const (
	LevelLow    Level = "low"
	LevelMedium Level = "medium"
	LevelHigh   Level = "high"
)

type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityMedium Priority = "medium"
	PriorityHigh   Priority = "high"
)

type Privacy string

const (
	PrivacyPublic  Privacy = "public"
	PrivacyPrivate Privacy = "private"
	PrivacySecret  Privacy = "secret"
)

type TaskType string

const (
	TaskTypeQA           TaskType = "qa"
	TaskTypeCoding       TaskType = "coding"
	TaskTypeRefactor     TaskType = "refactor"
	TaskTypeReview       TaskType = "review"
	TaskTypeArchitecture TaskType = "architecture"
	TaskTypeResearch     TaskType = "research"
	TaskTypeCreative     TaskType = "creative"
)

type StrategyName string

// TaskProfile describe una Task para que el router decida modelo y estrategia.
// En Fase 2 solo se llenan Type y Complexity; el resto queda con sus defaults.
type TaskProfile struct {
	Type        TaskType
	Complexity  Level
	Reasoning   Level
	ContextSize Level
	Multimodal  bool
	Latency     Priority
	Privacy     Privacy
	CostBudget  Priority
	Confidence  float64
}

type Constraints struct {
	MaxCostUSD      float64
	MaxTokens       int
	MaxDuration     time.Duration
	MaxRounds       int
	AllowCloud      bool
	AllowLocal      bool
	RequireJudgment bool
}

// Task es la unidad de trabajo del sistema (P2: todo es una Task).
type Task struct {
	ID        TaskID
	SessionID SessionID
	ParentID  *TaskID

	Prompt   string
	Messages []Message
	Profile  TaskProfile

	Strategy    StrategyName // vacío = decide el orchestrator
	Constraints Constraints

	CreatedAt time.Time
	Metadata  map[string]any
}
