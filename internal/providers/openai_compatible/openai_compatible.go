// Package openai_compatible implementa providers.Provider contra
// cualquier endpoint compatible con la API de chat completions de
// OpenAI: OpenRouter, vLLM, LM Studio, llama.cpp server, Groq,
// Together, etc. UN provider parametrizado (BaseURL + Headers), no uno
// por cada endpoint -- sección 21, Parte B punto 7.
package openaicompatible

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/artumarinn/arg0s/internal/core"
)

type Config struct {
	Name    string // nombre a reportar (ej "openrouter", "custom_openai")
	BaseURL string
	APIKey  string
	Headers map[string]string // headers extra (ej HTTP-Referer/X-Title de OpenRouter)
	HTTP    *http.Client
}

type Provider struct {
	name    string
	baseURL string
	apiKey  string
	headers map[string]string
	http    *http.Client
}

func New(cfg Config) *Provider {
	name := cfg.Name
	if name == "" {
		name = "openai_compatible"
	}
	client := cfg.HTTP
	if client == nil {
		client = &http.Client{}
	}
	return &Provider{
		name:    name,
		baseURL: strings.TrimSuffix(cfg.BaseURL, "/"),
		apiKey:  cfg.APIKey,
		headers: cfg.Headers,
		http:    client,
	}
}

func (p *Provider) Name() string { return p.name }

type modelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

func (p *Provider) Models(ctx context.Context) ([]core.Model, error) {
	resp, err := p.do(ctx, http.MethodGet, "/models", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := classifyStatus(p.name, resp); err != nil {
		return nil, err
	}

	var mr modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
		return nil, fmt.Errorf("%s: decode /models: %w", p.name, err)
	}

	models := make([]core.Model, 0, len(mr.Data))
	for _, m := range mr.Data {
		models = append(models, core.Model{ID: m.ID, Provider: p.name, ProviderModelID: m.ID, Local: false})
	}
	return models, nil
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type choice struct {
	Message      message `json:"message"`
	Delta        message `json:"delta"`
	FinishReason string  `json:"finish_reason"`
}

type chatResponse struct {
	Choices []choice `json:"choices"`
	Usage   *usage   `json:"usage"`
}

func toUsage(u *usage) core.Usage {
	if u == nil {
		return core.Usage{}
	}
	return core.Usage{InputTokens: u.PromptTokens, OutputTokens: u.CompletionTokens}
}

func (p *Provider) Complete(ctx context.Context, req core.Request) (core.Response, error) {
	body, err := json.Marshal(chatRequest{
		Model:    req.ModelID,
		Messages: []message{{Role: "user", Content: req.Prompt}},
		Stream:   false,
	})
	if err != nil {
		return core.Response{}, fmt.Errorf("%s: encode request: %w", p.name, err)
	}

	resp, err := p.do(ctx, http.MethodPost, "/chat/completions", body)
	if err != nil {
		return core.Response{}, err
	}
	defer resp.Body.Close()
	if err := classifyStatus(p.name, resp); err != nil {
		return core.Response{}, err
	}

	var cr chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return core.Response{}, fmt.Errorf("%s: decode response: %w", p.name, err)
	}

	var content, finishReason string
	if len(cr.Choices) > 0 {
		content = cr.Choices[0].Message.Content
		finishReason = cr.Choices[0].FinishReason
	}

	return core.Response{ModelID: req.ModelID, Content: content, Usage: toUsage(cr.Usage), FinishReason: finishReason}, nil
}

func (p *Provider) Stream(ctx context.Context, req core.Request) (<-chan core.Chunk, error) {
	body, err := json.Marshal(chatRequest{
		Model:    req.ModelID,
		Messages: []message{{Role: "user", Content: req.Prompt}},
		Stream:   true,
	})
	if err != nil {
		return nil, fmt.Errorf("%s: encode request: %w", p.name, err)
	}

	resp, err := p.do(ctx, http.MethodPost, "/chat/completions", body)
	if err != nil {
		return nil, err
	}
	if err := classifyStatus(p.name, resp); err != nil {
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
			line := scanner.Text()
			data, ok := strings.CutPrefix(line, "data: ")
			if !ok || data == "" {
				continue
			}
			if data == "[DONE]" {
				select {
				case ch <- core.Chunk{Done: true}:
				case <-ctx.Done():
				}
				return
			}

			var cr chatResponse
			if err := json.Unmarshal([]byte(data), &cr); err != nil {
				select {
				case ch <- core.Chunk{Err: fmt.Errorf("%s: decode stream event: %w", p.name, err)}:
				case <-ctx.Done():
				}
				return
			}

			var delta string
			if len(cr.Choices) > 0 {
				delta = cr.Choices[0].Delta.Content
			}
			out := core.Chunk{Delta: delta}
			if cr.Usage != nil {
				u := toUsage(cr.Usage)
				out.Usage = &u
			}
			select {
			case ch <- out:
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil {
			select {
			case ch <- core.Chunk{Err: fmt.Errorf("%s: read stream: %w", p.name, err)}:
			case <-ctx.Done():
			}
		}
	}()
	return ch, nil
}

func (p *Provider) do(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, reader)
	if err != nil {
		return nil, fmt.Errorf("%s: build request: %w", p.name, err)
	}
	if body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	for k, v := range p.headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := p.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%s: request failed: %s: %w", p.name, core.RedactSecrets(err.Error()), core.ErrRetryable)
	}
	return resp, nil
}

func classifyStatus(name string, resp *http.Response) error {
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	msg := core.RedactSecrets(string(body))

	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return &core.RateLimitError{RetryAfter: retryAfter(resp)}
	case resp.StatusCode >= 500:
		return fmt.Errorf("%s: %s: %s: %w", name, resp.Status, msg, core.ErrRetryable)
	default:
		return fmt.Errorf("%s: %s: %s", name, resp.Status, msg)
	}
}

func retryAfter(resp *http.Response) time.Duration {
	v := resp.Header.Get("Retry-After")
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil {
		return time.Duration(secs) * time.Second
	}
	return 0
}
