package contextc

import "sort"

// Rank fusiona fragmentos de TODAS las fuentes por relevancia
// descendente (sección 9: "ranking por relevancia (score por
// fuente)"). Cada fuente ya puntuó los suyos con su propio criterio
// (lexical para memory, match exacto/hint para codegraph, fijo para
// file) -- acá solo se ordena el conjunto combinado, no se
// re-normaliza entre fuentes (normalizar puntajes de criterios
// distintos sin una base común sería inventar precisión que no existe).
func Rank(fragments []Fragment) []Fragment {
	out := append([]Fragment(nil), fragments...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Relevance > out[j].Relevance })
	return out
}
