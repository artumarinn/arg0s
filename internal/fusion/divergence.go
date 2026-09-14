// Package fusion implementa el ensemble adaptativo de sección 10:
// correr un modelo primario, y solo escalar a secundarios en paralelo
// si hace falta -- la palabra clave es ADAPTATIVA, el default nunca es
// "correr N modelos siempre" (eso sería "1 request = 8 llamadas",
// justo lo que sección 10.1 dice que hay que evitar).
package fusion

import "strings"

// Divergence es el resultado de comparar la respuesta del primario
// contra los secundarios (sección 10.2).
type Divergence struct {
	Score  float64 // 0 = idénticas, 1 = completamente distintas
	Method string
}

// Measure calcula la divergencia entre primary y CADA secondary, y
// devuelve el PEOR caso (máximo) -- un solo secundario que discrepa
// fuerte ya es señal de desacuerdo real, promediarlo con secundarios
// que sí coinciden lo escondería.
//
// method soporta hoy "lexical" de verdad. "semantic"/"judge"/"hybrid"
// (sección 10.2, tabla de métodos) degradan a lexical -- no hay
// embeddings locales ni Judgment Day todavía (Fase 5); mejor decir la
// verdad de lo que hace que fingir un método que no existe.
func Measure(method, primary string, secondaries []string) Divergence {
	if len(secondaries) == 0 {
		return Divergence{Score: 0, Method: "lexical"}
	}

	worst := 0.0
	for _, s := range secondaries {
		d := lexicalDivergence(primary, s)
		if d > worst {
			worst = d
		}
	}
	return Divergence{Score: worst, Method: "lexical"}
}

// lexicalDivergence es 1 - similitud. La similitud usa normalización
// estructural cuando el contenido tiene pinta de código (sección 10.2:
// "para código, comparar estructura normalizada, no texto crudo -- dos
// implementaciones iguales con nombres distintos NO son divergencia").
// Para texto libre, usa similitud lexical simple (Jaccard de tokens).
func lexicalDivergence(a, b string) float64 {
	if looksLikeCode(a) && looksLikeCode(b) {
		a, b = normalizeCode(a), normalizeCode(b)
	}
	return 1 - jaccardSimilarity(tokenize(a), tokenize(b))
}

// looksLikeCode es una heurística barata -- no un detector de
// lenguaje real. Alcanza para decidir "vale la pena normalizar
// identificadores acá" sin parsear nada.
func looksLikeCode(s string) bool {
	return strings.ContainsAny(s, "{};") || strings.Contains(s, "func ") || strings.Contains(s, "def ") || strings.Contains(s, "class ")
}

// normalizeCode canoniza identificadores a placeholders posicionales
// (v1, v2, ...) según el orden de primera aparición -- dos funciones
// estructuralmente iguales con variables renombradas producen el MISMO
// stream canonizado, así que su divergencia lexical cae a ~0. No es un
// parser real (no distingue tipo/scope), es una heurística de una sola
// pasada sobre tokens -- alcanza para el caso que sección 10.2 pide
// explícitamente sin escribir un parser multi-lenguaje.
func normalizeCode(s string) string {
	tokens := tokenize(s)
	seen := map[string]string{}
	var out []string
	for _, t := range tokens {
		if isKeywordOrSymbol(t) {
			out = append(out, t)
			continue
		}
		canon, ok := seen[t]
		if !ok {
			canon = "v" + itoa(len(seen)+1)
			seen[t] = canon
		}
		out = append(out, canon)
	}
	return strings.Join(out, " ")
}

// goKeywords son las palabras que NO se canonizan -- si se reemplazan
// "func"/"return"/etc por v1/v2, dos funciones con estructura de
// control DISTINTA (un if de más) podrían terminar pareciendo iguales.
// Lista corta a propósito: cubre Go/C-like, no todos los lenguajes --
// documentado como heurística, sección de arriba.
var goKeywords = map[string]bool{
	"func": true, "return": true, "if": true, "else": true, "for": true,
	"range": true, "var": true, "const": true, "type": true, "struct": true,
	"interface": true, "package": true, "import": true, "def": true, "class": true,
	"import_": true,
}

func isKeywordOrSymbol(t string) bool {
	if goKeywords[t] {
		return true
	}
	for _, r := range t {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return true // tiene algún símbolo (paréntesis, llave, punto...) -- no es un identificador puro
	}
	return false
}

func tokenize(s string) []string {
	return strings.Fields(s)
}

func jaccardSimilarity(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1
	}
	setA := toSet(a)
	setB := toSet(b)
	inter := 0
	for t := range setA {
		if setB[t] {
			inter++
		}
	}
	union := len(setA) + len(setB) - inter
	if union == 0 {
		return 1
	}
	return float64(inter) / float64(union)
}

func toSet(items []string) map[string]bool {
	set := map[string]bool{}
	for _, it := range items {
		set[it] = true
	}
	return set
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
