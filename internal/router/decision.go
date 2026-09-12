package router

import "github.com/artumarinn/arg0s/internal/core"

// RoutingDecision es siempre auditable -- sección 8: "poder responder
// por qué eligió ese modelo es un requisito, no un extra". Se persiste
// completa (runs.routing) y es lo que imprime `arg0s route explain`.
type RoutingDecision struct {
	TaskID               string          `json:"task_id"`
	Profile              ProfileSummary  `json:"profile"`
	Mode                 string          `json:"mode"`
	ClassifierConfidence float64         `json:"classifier_confidence,omitempty"`
	Candidates           []string        `json:"candidates"`
	SelectedModel        string          `json:"selected_model"`
	SelectedStrategy     string          `json:"selected_strategy"`
	Reasons              []string        `json:"reasons"`
	Rejected             []RejectedModel `json:"rejected,omitempty"`
}

// ProfileSummary es el recorte de TaskProfile que entra en el JSON de
// la decisión -- los campos con confianza real en Fase 2.
type ProfileSummary struct {
	Type       core.TaskType `json:"type"`
	Complexity core.Level    `json:"complexity"`
	Privacy    core.Privacy  `json:"privacy"`
}

type RejectedModel struct {
	Model  string `json:"model"`
	Reason string `json:"reason"`
}

func summarize(p core.TaskProfile) ProfileSummary {
	return ProfileSummary{Type: p.Type, Complexity: p.Complexity, Privacy: p.Privacy}
}
