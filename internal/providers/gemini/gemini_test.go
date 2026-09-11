package gemini

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

func TestModels_StripsModelsPrefix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "test-key", r.URL.Query().Get("key"))
		fmt.Fprint(w, `{"models":[{"name":"models/gemini-2.5-flash"},{"name":"models/gemini-2.5-pro"}]}`)
	}))
	defer srv.Close()

	p := New(Config{BaseURL: srv.URL, APIKey: "test-key"})
	models, err := p.Models(context.Background())

	require.NoError(t, err)
	require.Len(t, models, 2)
	require.Equal(t, "gemini-2.5-flash", models[0].ID)
}

func TestComplete_ParsesTextAndUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/models/gemini-2.5-flash:generateContent", r.URL.Path)
		fmt.Fprint(w, `{"candidates":[{"content":{"parts":[{"text":"un mutex es un candado"}]}}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":8}}`)
	}))
	defer srv.Close()

	p := New(Config{BaseURL: srv.URL, APIKey: "test-key"})
	resp, err := p.Complete(context.Background(), core.Request{ModelID: "gemini-2.5-flash", Prompt: "qué es un mutex"})

	require.NoError(t, err)
	require.Equal(t, "un mutex es un candado", resp.Content)
	require.Equal(t, 5, resp.Usage.InputTokens)
	require.Equal(t, 8, resp.Usage.OutputTokens)
}

func TestComplete_5xxIsRetryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, "overloaded")
	}))
	defer srv.Close()

	p := New(Config{BaseURL: srv.URL, APIKey: "test-key"})
	_, err := p.Complete(context.Background(), core.Request{ModelID: "gemini-2.5-flash", Prompt: "hola"})

	require.Error(t, err)
	require.ErrorIs(t, err, core.ErrRetryable)
}

func TestComplete_401IsNotRetryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "invalid api key")
	}))
	defer srv.Close()

	p := New(Config{BaseURL: srv.URL, APIKey: "test-key"})
	_, err := p.Complete(context.Background(), core.Request{ModelID: "gemini-2.5-flash", Prompt: "hola"})

	require.Error(t, err)
	require.NotErrorIs(t, err, core.ErrRetryable)
}

func TestComplete_429ReturnsRateLimitWithRetryAfter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "3")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	p := New(Config{BaseURL: srv.URL, APIKey: "test-key"})
	_, err := p.Complete(context.Background(), core.Request{ModelID: "gemini-2.5-flash", Prompt: "hola"})

	var rlErr *core.RateLimitError
	require.ErrorAs(t, err, &rlErr)
	require.Equal(t, 3*time.Second, rlErr.RetryAfter)
}

func TestComplete_NetworkErrorNeverLeaksAPIKeyInURL(t *testing.T) {
	// Puerto cerrado en loopback -- el *url.Error de net/http incluye
	// la URL completa (con ?key=...) en su Error(). Confirmamos que la
	// key no sobrevive.
	const fakeKey = "AIzaSyABCDEFGHIJKLMNOPQRSTUVWXYZ0123456"
	p := New(Config{BaseURL: "http://127.0.0.1:1", APIKey: fakeKey, HTTP: &http.Client{Timeout: 2 * time.Second}})

	_, err := p.Complete(context.Background(), core.Request{ModelID: "gemini-2.5-flash", Prompt: "hola"})

	require.Error(t, err)
	require.ErrorIs(t, err, core.ErrRetryable)
	require.NotContains(t, err.Error(), fakeKey)
}

func TestStream_ParsesSSEEventsAndFinalUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "sse", r.URL.Query().Get("alt"))
		flusher := w.(http.Flusher)
		events := []string{
			`{"candidates":[{"content":{"parts":[{"text":"un "}]}}]}`,
			`{"candidates":[{"content":{"parts":[{"text":"mutex"}]}}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":2}}`,
		}
		for _, e := range events {
			fmt.Fprintf(w, "data: %s\n\n", e)
			flusher.Flush()
		}
	}))
	defer srv.Close()

	p := New(Config{BaseURL: srv.URL, APIKey: "test-key"})
	ch, err := p.Stream(context.Background(), core.Request{ModelID: "gemini-2.5-flash", Prompt: "hola"})
	require.NoError(t, err)

	var full strings.Builder
	var finalUsage *core.Usage
	for c := range ch {
		require.NoError(t, c.Err)
		full.WriteString(c.Delta)
		if c.Usage != nil {
			finalUsage = c.Usage
		}
	}

	require.Equal(t, "un mutex", full.String())
	require.NotNil(t, finalUsage)
	require.Equal(t, 3, finalUsage.InputTokens)
	require.Equal(t, 2, finalUsage.OutputTokens)
}
