package core

import "time"

// Artifact es una salida estructurada de un Run: patch, archivo, JSON.
type Artifact struct {
	ID        string
	RunID     string
	Kind      string // patch | file | json | report
	Path      string
	Content   string
	Hash      string
	CreatedAt time.Time
}
