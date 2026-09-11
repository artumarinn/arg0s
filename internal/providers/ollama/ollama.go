// Package ollama implementa providers.Provider contra un servidor Ollama
// local. Sin API key -- es local y gratis (por eso va primero de los
// providers reales, sección 21).
package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/artumarinn/arg0s/internal/core"
)

// Config son los parámetros de este provider — no viene de
// internal/config directamente (ver ADR de desacople providers/config).
type Config struct {
	BaseURL string // default http://localhost:11434
	HTTP    *http.Client
}

type Provider struct {
	baseURL string
	http    *http.Client
}

func New(cfg Config) *Provider {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	client := cfg.HTTP
	if client == nil {
		client = &http.Client{}
	}
	return &Provider{baseURL: baseURL, http: client}
}

func (p *Provider) Name() string { return "ollama" }

type tagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

func (p *Provider) Models(ctx context.Context) ([]core.Model, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/api/tags", nil)
	if err != nil {
		return nil, fmt.Errorf("ollama: build request: %w", err)
	}
	resp, err := p.http.Do(httpReq)
	if err != nil {
		return nil, classifyNetErr(err)
	}
	defer resp.Body.Close()

	if err := classifyStatus(resp); err != nil {
		return nil, err
	}

	var tags tagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return nil, fmt.Errorf("ollama: decode /api/tags: %w", err)
	}

	models := make([]core.Model, 0, len(tags.Models))
	for _, m := range tags.Models {
		models = append(models, core.Model{ID: m.Name, Provider: "ollama", ProviderModelID: m.Name, Local: true})
	}
	return models, nil
}

type generateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type generateChunk struct {
	Response      string `json:"response"`
	Done          bool   `json:"done"`
	PromptEvalCnt int    `json:"prompt_eval_count"`
	EvalCount     int    `json:"eval_count"`
	ErrorField    string `json:"error"`
}

func (p *Provider) Complete(ctx context.Context, req core.Request) (core.Response, error) {
	body, err := json.Marshal(generateRequest{Model: req.ModelID, Prompt: req.Prompt, Stream: false})
	if err != nil {
		return core.Response{}, fmt.Errorf("ollama: encode request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return core.Response{}, fmt.Errorf("ollama: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.http.Do(httpReq)
	if err != nil {
		return core.Response{}, classifyNetErr(err)
	}
	defer resp.Body.Close()

	if err := classifyStatus(resp); err != nil {
		return core.Response{}, err
	}

	var chunk generateChunk
	if err := json.NewDecoder(resp.Body).Decode(&chunk); err != nil {
		return core.Response{}, fmt.Errorf("ollama: decode response: %w", err)
	}
	if chunk.ErrorField != "" {
		return core.Response{}, fmt.Errorf("ollama: %s", chunk.ErrorField)
	}

	return core.Response{
		ModelID: req.ModelID,
		Content: chunk.Response,
		Usage: core.Usage{
			InputTokens:  chunk.PromptEvalCnt,
			OutputTokens: chunk.EvalCount,
		},
	}, nil
}

func (p *Provider) Stream(ctx context.Context, req core.Request) (<-chan core.Chunk, error) {
	body, err := json.Marshal(generateRequest{Model: req.ModelID, Prompt: req.Prompt, Stream: true})
	if err != nil {
		return nil, fmt.Errorf("ollama: encode request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.http.Do(httpReq)
	if err != nil {
		return nil, classifyNetErr(err)
	}
	if err := classifyStatus(resp); err != nil {
		resp.Body.Close()
		return nil, err
	}

	ch := make(chan core.Chunk)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}
			var c generateChunk
			if err := json.Unmarshal(line, &c); err != nil {
				select {
				case ch <- core.Chunk{Err: fmt.Errorf("ollama: decode stream line: %w", err)}:
				case <-ctx.Done():
				}
				return
			}
			if c.ErrorField != "" {
				select {
				case ch <- core.Chunk{Err: fmt.Errorf("ollama: %s", c.ErrorField)}:
				case <-ctx.Done():
				}
				return
			}

			out := core.Chunk{Delta: c.Response, Done: c.Done}
			if c.Done {
				out.Usage = &core.Usage{InputTokens: c.PromptEvalCnt, OutputTokens: c.EvalCount}
			}
			select {
			case ch <- out:
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil && err != io.EOF {
			select {
			case ch <- core.Chunk{Err: fmt.Errorf("ollama: read stream: %w", err)}:
			case <-ctx.Done():
			}
		}
	}()
	return ch, nil
}

// classifyStatus clasifica el status HTTP: 5xx retryable, 429 rate
// limit (Ollama no lo emite en la práctica, pero cubrimos el contrato
// genérico de status), el resto se reporta con el body como contenido.
func classifyStatus(resp *http.Response) error {
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	msg := core.RedactSecrets(string(body))

	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return &core.RateLimitError{}
	case resp.StatusCode >= 500:
		return fmt.Errorf("ollama: %s: %s: %w", resp.Status, msg, core.ErrRetryable)
	default:
		return fmt.Errorf("ollama: %s: %s", resp.Status, msg)
	}
}

// classifyNetErr envuelve errores de red (conexión rechazada, DNS,
// etc) como retryable -- son transitorios por naturaleza (ollama
// todavía no arrancó, se reinicia, etc). Si el error de fondo es
// context.DeadlineExceeded/Canceled, el %w múltiple lo preserva —
// errors.Is sigue encontrándolo, y el executor distingue timeout de
// provider vs ctx del caller mirando el ctx, no este wrap.
func classifyNetErr(err error) error {
	return fmt.Errorf("ollama: %w: %w", err, core.ErrRetryable)
}
