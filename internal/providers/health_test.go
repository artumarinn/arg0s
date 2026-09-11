package providers_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/artumarinn/arg0s/internal/core"
	"github.com/artumarinn/arg0s/internal/providers"
	"github.com/artumarinn/arg0s/internal/providers/mock"
)

// fakeModelsErr sabe fallar Models() sin implementar todo Provider a
// mano -- envuelve el mock y le pisa Models().
type fakeModelsErr struct {
	*mock.Provider
	name string
}

func (f fakeModelsErr) Name() string { return f.name }
func (f fakeModelsErr) Models(ctx context.Context) ([]core.Model, error) {
	return nil, context.DeadlineExceeded
}

func TestHealth_ReportsPassAndSkipOnError(t *testing.T) {
	reg := providers.NewRegistry()
	reg.Register(mock.New(mock.Config{Name: "ollama", Models: []core.Model{{ID: "m1"}, {ID: "m2"}}}))
	reg.Register(fakeModelsErr{Provider: mock.New(mock.Config{}), name: "gemini"})

	checks := reg.Health(context.Background(), time.Second)

	require.Len(t, checks, 2)
	byName := map[string]core.Check{}
	for _, c := range checks {
		byName[c.Name] = c
	}

	require.Equal(t, core.CheckPass, byName["ollama connectivity"].Status)
	require.Contains(t, byName["ollama connectivity"].Detail, "2 models loaded")

	require.Equal(t, core.CheckSkip, byName["gemini connectivity"].Status)
}
