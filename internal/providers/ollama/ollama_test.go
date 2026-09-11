package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/artumarinn/arg0s/internal/core"
)

// httptest.Server corre en loopback local -- no es "red" en el sentido
// de la regla dura #8 (nunca tocar la red real). Sin esto no se puede
// testear el parsing de las respuestas sin levantar Ollama de verdad.

func TestModels_ParsesTagsResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/tags", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]string{{"name": "qwen2.5-coder:7b"}, {"name": "qwen2.5-coder:14b"}},
		})
	}))
	defer srv.Close()

	p := New(Config{BaseURL: srv.URL})
	models, err := p.Models(context.Background())

	require.NoError(t, err)
	require.Len(t, models, 2)
	require.Equal(t, "qwen2.5-coder:7b", models[0].ID)
	require.True(t, models[0].Local)
}

func TestComplete_ParsesResponseAndUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/generate", r.URL.Path)
		var body generateRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "qwen2.5-coder:7b", body.Model)
		require.False(t, body.Stream)

		_ = json.NewEncoder(w).Encode(generateChunk{
			Response: "un mutex es un candado", Done: true, PromptEvalCnt: 5, EvalCount: 8,
		})
	}))
	defer srv.Close()

	p := New(Config{BaseURL: srv.URL})
	resp, err := p.Complete(context.Background(), core.Request{ModelID: "qwen2.5-coder:7b", Prompt: "qué es un mutex"})

	require.NoError(t, err)
	require.Equal(t, "un mutex es un candado", resp.Content)
	require.Equal(t, 5, resp.Usage.InputTokens)
	require.Equal(t, 8, resp.Usage.OutputTokens)
}

func TestComplete_5xxIsRetryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	p := New(Config{BaseURL: srv.URL})
	_, err := p.Complete(context.Background(), core.Request{ModelID: "x", Prompt: "hola"})

	require.Error(t, err)
	require.ErrorIs(t, err, core.ErrRetryable)
}

func TestComplete_4xxIsNotRetryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("model not found"))
	}))
	defer srv.Close()

	p := New(Config{BaseURL: srv.URL})
	_, err := p.Complete(context.Background(), core.Request{ModelID: "no-existe", Prompt: "hola"})

	require.Error(t, err)
	require.NotErrorIs(t, err, core.ErrRetryable)
}

func TestComplete_ConnectionRefusedIsRetryable(t *testing.T) {
	// Puerto cerrado en loopback -- connection refused, no red real.
	p := New(Config{BaseURL: "http://127.0.0.1:1", HTTP: &http.Client{Timeout: 2 * time.Second}})
	_, err := p.Complete(context.Background(), core.Request{ModelID: "x", Prompt: "hola"})

	require.Error(t, err)
	require.ErrorIs(t, err, core.ErrRetryable)
}

func TestStream_EmitsChunksAndFinalUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body generateRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.True(t, body.Stream)

		flusher := w.(http.Flusher)
		lines := []generateChunk{
			{Response: "un ", Done: false},
			{Response: "mutex", Done: false},
			{Done: true, PromptEvalCnt: 3, EvalCount: 2},
		}
		for _, l := range lines {
			_ = json.NewEncoder(w).Encode(l)
			flusher.Flush()
		}
	}))
	defer srv.Close()

	p := New(Config{BaseURL: srv.URL})
	ch, err := p.Stream(context.Background(), core.Request{ModelID: "x", Prompt: "hola"})
	require.NoError(t, err)

	var full string
	var finalUsage *core.Usage
	for c := range ch {
		require.NoError(t, c.Err)
		full += c.Delta
		if c.Usage != nil {
			finalUsage = c.Usage
		}
	}

	require.Equal(t, "un mutex", full)
	require.NotNil(t, finalUsage)
	require.Equal(t, 3, finalUsage.InputTokens)
	require.Equal(t, 2, finalUsage.OutputTokens)
}

func TestComplete_RespectsContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond) // más que el deadline del cliente
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := New(Config{BaseURL: srv.URL})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := p.Complete(ctx, core.Request{ModelID: "x", Prompt: "hola"})
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}
