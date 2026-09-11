package openaicompatible

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/artumarinn/arg0s/internal/core"
)

func TestComplete_SendsAuthHeaderAndExtraHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer sk-test-123", r.Header.Get("Authorization"))
		require.Equal(t, "arg0s", r.Header.Get("X-Title"))
		fmt.Fprint(w, `{"choices":[{"message":{"content":"una respuesta"},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":6}}`)
	}))
	defer srv.Close()

	p := New(Config{Name: "openrouter", BaseURL: srv.URL, APIKey: "sk-test-123", Headers: map[string]string{"X-Title": "arg0s"}})
	resp, err := p.Complete(context.Background(), core.Request{ModelID: "some-model", Prompt: "hola"})

	require.NoError(t, err)
	require.Equal(t, "una respuesta", resp.Content)
	require.Equal(t, 4, resp.Usage.InputTokens)
	require.Equal(t, 6, resp.Usage.OutputTokens)
}

func TestModels_ParsesDataArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[{"id":"meta-llama/llama-3-70b"},{"id":"mistralai/mixtral-8x7b"}]}`)
	}))
	defer srv.Close()

	p := New(Config{Name: "openrouter", BaseURL: srv.URL, APIKey: "sk-test"})
	models, err := p.Models(context.Background())

	require.NoError(t, err)
	require.Len(t, models, 2)
	require.Equal(t, "meta-llama/llama-3-70b", models[0].ID)
}

func TestComplete_5xxIsRetryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprint(w, "upstream error")
	}))
	defer srv.Close()

	p := New(Config{BaseURL: srv.URL, APIKey: "sk-test"})
	_, err := p.Complete(context.Background(), core.Request{ModelID: "x", Prompt: "hola"})

	require.Error(t, err)
	require.ErrorIs(t, err, core.ErrRetryable)
}

func TestComplete_401IsNotRetryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "invalid key")
	}))
	defer srv.Close()

	p := New(Config{BaseURL: srv.URL, APIKey: "sk-test"})
	_, err := p.Complete(context.Background(), core.Request{ModelID: "x", Prompt: "hola"})

	require.Error(t, err)
	require.NotErrorIs(t, err, core.ErrRetryable)
}

func TestComplete_429ReturnsRateLimitWithRetryAfter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "2")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	p := New(Config{BaseURL: srv.URL, APIKey: "sk-test"})
	_, err := p.Complete(context.Background(), core.Request{ModelID: "x", Prompt: "hola"})

	var rlErr *core.RateLimitError
	require.ErrorAs(t, err, &rlErr)
	require.Equal(t, 2*time.Second, rlErr.RetryAfter)
}

func TestComplete_NeverLeaksAPIKeyOnNetworkError(t *testing.T) {
	const fakeKey = "sk-or-v1-ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	p := New(Config{BaseURL: "http://127.0.0.1:1", APIKey: fakeKey, HTTP: &http.Client{Timeout: 2 * time.Second}})

	_, err := p.Complete(context.Background(), core.Request{ModelID: "x", Prompt: "hola"})

	require.Error(t, err)
	require.ErrorIs(t, err, core.ErrRetryable)
	require.NotContains(t, err.Error(), fakeKey)
}

func TestStream_ParsesSSEAndDoneMarker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher := w.(http.Flusher)
		events := []string{
			`{"choices":[{"delta":{"content":"un "}}]}`,
			`{"choices":[{"delta":{"content":"mutex"}}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`,
		}
		for _, e := range events {
			fmt.Fprintf(w, "data: %s\n\n", e)
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer srv.Close()

	p := New(Config{BaseURL: srv.URL, APIKey: "sk-test"})
	ch, err := p.Stream(context.Background(), core.Request{ModelID: "x", Prompt: "hola"})
	require.NoError(t, err)

	var full strings.Builder
	var sawDone bool
	var finalUsage *core.Usage
	for c := range ch {
		require.NoError(t, c.Err)
		full.WriteString(c.Delta)
		if c.Usage != nil {
			finalUsage = c.Usage
		}
		if c.Done {
			sawDone = true
		}
	}

	require.Equal(t, "un mutex", full.String())
	require.True(t, sawDone)
	require.NotNil(t, finalUsage)
	require.Equal(t, 3, finalUsage.InputTokens)
}
