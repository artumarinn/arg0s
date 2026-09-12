// Package router elige modelo y estrategia para una Task sin
// intervención del usuario, y deja registrado por qué (sección 8 de
// docs/ARG0S.md). En Fase 2 el modo default es heuristic; classifier y
// hybrid existen pero quedan detrás de router.mode -- no default,
// porque dependen de un modelo classifier que todavía no se probó en
// producción.
package router

import (
	"os/exec"
	"strings"

	"github.com/artumarinn/arg0s/internal/core"
)

// heuristicConfidence es fija para el modo heuristic -- no es un score
// aprendido, es "qué tan seguras son las reglas de sección 8" en
// abstracto. Sirve de umbral para el modo hybrid (router.
// classifier_threshold).
const heuristicConfidence = 0.7

// ProfileHeuristic perfila una Task con las reglas de sección 8. Solo
// Type y Complexity tienen confianza real en Fase 2; el resto queda en
// defaults documentados (Reasoning/ContextSize/Latency/CostBudget:
// medium -- confianza real llega en fases posteriores). Privacy es una
// señal real adicional (git remote + policies.yaml), no un default.
func ProfileHeuristic(prompt, cwd string) core.TaskProfile {
	return core.TaskProfile{
		Type:        classifyType(prompt),
		Complexity:  classifyComplexity(prompt),
		Reasoning:   core.LevelMedium,
		ContextSize: core.LevelMedium,
		Multimodal:  false,
		Latency:     core.PriorityMedium,
		Privacy:     classifyPrivacy(cwd),
		CostBudget:  core.PriorityMedium,
		Confidence:  heuristicConfidence,
	}
}

// classifyComplexity sigue las reglas de sección 8 al pie de la letra.
func classifyComplexity(prompt string) core.Level {
	n := len(prompt)
	lower := strings.ToLower(prompt)

	if n > 1000 || containsAny(lower, "arquitectura", "refactor", "seguridad", "diseño de sistema", "diseña un sistema") || countFileMentions(lower) > 3 {
		return core.LevelHigh
	}
	if n >= 200 || mentionsFile(lower) || containsAny(lower, "código", "codigo", "implementa", "escribe una función", "escribe un") {
		return core.LevelMedium
	}
	return core.LevelLow
}

// classifyType matchea los keywords de sección 8, en el orden ahí
// listado. Sin match, default a qa -- es el tipo más neutral.
func classifyType(prompt string) core.TaskType {
	lower := strings.ToLower(prompt)
	switch {
	case containsAny(lower, "refactor", "refactoriza"):
		return core.TaskTypeRefactor
	case containsAny(lower, "revisa", "review", "audita"):
		return core.TaskTypeReview
	case containsAny(lower, "diseña", "arquitectura"):
		return core.TaskTypeArchitecture
	case containsAny(lower, "implementa", "escribe"):
		return core.TaskTypeCoding
	case containsAny(lower, "qué", "que ", "cómo", "como ", "por qué", "por que", "cuánto", "cuanto"):
		return core.TaskTypeQA
	default:
		return core.TaskTypeQA
	}
}

// classifyPrivacy es private si cwd es un repo git con al menos un
// remote configurado -- no podemos saber si el remote es público o
// privado sin llamar a la API de GitHub (fuera de alcance acá), así
// que "tiene remote" ya es tratado como señal de privacidad.
//
// secret_paths (policies.yaml) queda con el hook sin implementar: el
// Policy Engine completo es de una fase posterior (ver CLAUDE.md).
// TODO(fase-policy-engine): leer ~/.arg0s/policies.yaml y matchear
// secret_paths contra cwd/archivos mencionados en el prompt.
func classifyPrivacy(cwd string) core.Privacy {
	if hasGitRemote(cwd) {
		return core.PrivacyPrivate
	}
	return core.PrivacyPublic
}

func hasGitRemote(cwd string) bool {
	cmd := exec.Command("git", "remote")
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

func containsAny(s string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}

func mentionsFile(lower string) bool {
	return countFileMentions(lower) > 0
}

// countFileMentions cuenta tokens con pinta de path de archivo
// (contienen "/" o una extensión conocida) -- heurística barata para
// "menciona archivos" y "involucra > 3 archivos" de sección 8.
func countFileMentions(lower string) int {
	exts := []string{".go", ".py", ".ts", ".tsx", ".js", ".jsx", ".yaml", ".yml", ".json", ".md", ".rs", ".java"}
	count := 0
	for _, tok := range strings.Fields(lower) {
		tok = strings.Trim(tok, ".,;:()[]\"'")
		if strings.Contains(tok, "/") {
			count++
			continue
		}
		for _, ext := range exts {
			if strings.HasSuffix(tok, ext) {
				count++
				break
			}
		}
	}
	return count
}
