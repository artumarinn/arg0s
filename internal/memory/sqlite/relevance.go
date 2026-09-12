package sqlite

import "strings"

// Relevance es lexical/keyword matching simple -- NO embeddings.
// Fase 3 lo deja así a propósito: "no embeddings todavía" está en los
// anti-objetivos de docs/ARG0S.md sección 20 hasta que se decida lo
// contrario explícitamente. Es la fracción de tokens de la query que
// aparecen en título+contenido (con el título contando doble: un
// match ahí es más fuerte señal que uno perdido en el contenido).
//
// Sin query (q vacío) toda memoria matchea al máximo -- "traer las
// últimas N del proyecto" sigue siendo un caso de uso válido.
func Relevance(query, title, content string) float64 {
	qTokens := tokenize(query)
	if len(qTokens) == 0 {
		return 1
	}

	titleTokens := tokenSet(title)
	contentTokens := tokenSet(content)

	var hits float64
	for _, t := range qTokens {
		switch {
		case titleTokens[t]:
			hits += 2
		case contentTokens[t]:
			hits += 1
		}
	}

	max := float64(len(qTokens)) * 2
	return hits / max
}

func tokenize(s string) []string {
	return strings.Fields(strings.ToLower(s))
}

func tokenSet(s string) map[string]bool {
	set := map[string]bool{}
	for _, t := range tokenize(s) {
		set[t] = true
	}
	return set
}
