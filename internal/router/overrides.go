package router

import (
	"fmt"

	"github.com/artumarinn/arg0s/internal/config"
	"github.com/artumarinn/arg0s/internal/core"
)

// appliedOverrides es el resultado de aplicar router.overrides contra
// un TaskProfile -- config-driven (sección 8), nunca hardcodeado acá.
type appliedOverrides struct {
	ForceTier     string
	ForceLocal    bool
	ForceStrategy string
	ForceModel    string
	Reasons       []string
}

// applyOverrides evalúa cfg.Overrides en orden; cada override que
// matchea acumula sus force_* (el último que setea un campo dado
// gana) y deja su propio reason. Sin overrides, no hace nada.
func applyOverrides(profile core.TaskProfile, overrides []config.OverrideConfig) appliedOverrides {
	var out appliedOverrides
	for _, o := range overrides {
		if !matches(o.When, profile) {
			continue
		}
		if o.ForceTier != "" {
			out.ForceTier = o.ForceTier
		}
		if o.ForceLocal {
			out.ForceLocal = true
		}
		if o.ForceStrategy != "" {
			out.ForceStrategy = o.ForceStrategy
		}
		if o.ForceModel != "" {
			out.ForceModel = o.ForceModel
		}
		out.Reasons = append(out.Reasons, fmt.Sprintf("override %s aplicado", describeWhen(o.When)))
	}
	return out
}

func matches(w config.OverrideWhen, profile core.TaskProfile) bool {
	if w.Type != "" && w.Type != string(profile.Type) {
		return false
	}
	if w.Complexity != "" && w.Complexity != string(profile.Complexity) {
		return false
	}
	if w.Privacy != "" && w.Privacy != string(profile.Privacy) {
		return false
	}
	return w.Type != "" || w.Complexity != "" || w.Privacy != ""
}

func describeWhen(w config.OverrideWhen) string {
	s := "{"
	sep := ""
	if w.Type != "" {
		s += sep + "type=" + w.Type
		sep = ", "
	}
	if w.Complexity != "" {
		s += sep + "complexity=" + w.Complexity
		sep = ", "
	}
	if w.Privacy != "" {
		s += sep + "privacy=" + w.Privacy
	}
	return s + "}"
}
