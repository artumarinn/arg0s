package execution

import "github.com/artumarinn/arg0s/internal/core"

// redactedError sanea el mensaje de error antes de que salga del
// executor (regla dura #4: las API keys nunca aparecen en errores).
// Preserva la cadena Unwrap para que errors.Is/errors.As sigan
// funcionando sobre el error original.
type redactedError struct {
	msg   string
	cause error
}

func (e *redactedError) Error() string { return e.msg }
func (e *redactedError) Unwrap() error { return e.cause }

func redactErr(err error) error {
	if err == nil {
		return nil
	}
	return &redactedError{msg: core.RedactSecrets(err.Error()), cause: err}
}
