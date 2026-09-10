package config

import "embed"

//go:embed templates/config.yaml templates/models.yaml
var templatesFS embed.FS

// DefaultConfigYAML es el config.yaml de la sección 6.3, verbatim —
// usado por `arg0s init` para materializar ~/.arg0s/config.yaml.
func DefaultConfigYAML() []byte {
	b, err := templatesFS.ReadFile("templates/config.yaml")
	if err != nil {
		panic("config: templates/config.yaml embebido falta: " + err.Error())
	}
	return b
}

// DefaultModelsYAML es el models.yaml de la sección 6.4, verbatim.
func DefaultModelsYAML() []byte {
	b, err := templatesFS.ReadFile("templates/models.yaml")
	if err != nil {
		panic("config: templates/models.yaml embebido falta: " + err.Error())
	}
	return b
}

// DefaultEnvTemplate es la plantilla de .env de la sección 6.5, con las
// keys vacías en vez del placeholder "...".
func DefaultEnvTemplate() []byte {
	return []byte(`# ~/.arg0s/.env — chmod 600, jamás en git
GEMINI_API_KEY=
OPENROUTER_API_KEY=
`)
}
