package core

import "regexp"

// secretPattern matchea tokens largos alfanuméricos — la forma típica
// de una API key (sk-..., AIza..., etc). Es heurístico a propósito:
// prefiere sobre-redactar antes que dejar pasar una key real.
var secretPattern = regexp.MustCompile(`[A-Za-z0-9_\-]{16,}`)

// RedactSecrets reemplaza tokens largos por su versión truncada
// (primeros 3 + "..." + últimos 4). Se aplica a todo texto que pueda
// llegar a logs, eventos o errores — nunca al contenido de una Response
// (eso es la respuesta real que pidió el usuario, no un canal de logs).
// Ver CLAUDE.md regla dura #4.
func RedactSecrets(s string) string {
	return secretPattern.ReplaceAllStringFunc(s, func(tok string) string {
		if len(tok) <= 10 {
			return tok
		}
		return tok[:3] + "..." + tok[len(tok)-4:]
	})
}
