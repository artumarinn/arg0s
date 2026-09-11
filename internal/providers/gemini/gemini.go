// Package gemini implementa providers.Provider contra la API de Gemini.
// La API key va SIEMPRE en query string (?key=...), nunca se construye
// un mensaje de error con la URL completa -- ver RedactSecrets y la
// regla dura #4.
package gemini

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
	BaseURL string // default https://generativelanguage.googleapis.com/v1beta
	APIKey  string
	HTTP    *http.Client
}

type Provider struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func New(cfg Config) *Provider {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com/v1beta"
	}
	client := cfg.HTTP
	if client == nil {
		client = &http.Client{}
	}
	return &Provider{baseURL: baseURL, apiKey: cfg.APIKey, http: client}
}

func (p *Provider) Name() string { return "gemini" }

type modelsResponse struct {
	Models []struct {
		Name string `json:"name"` // "models/gemini-2.5-flash"
	} `json:"models"`
}

func (p *Provider) Models(ctx context.Context) ([]core.Model, error) {
	resp, err := p.do(ctx, http.MethodGet, "/models", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := classifyStatus(resp); err != nil {
		return nil, err
	}

	var mr modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
		return nil, fmt.Errorf("gemini: decode /models: %w", err)
	}

	models := make([]core.Model, 0, len(mr.Models))
	for _, m := range mr.Models {
		id := strings.TrimPrefix(m.Name, "models/")
		models = append(models, core.Model{ID: id, Provider: "gemini", ProviderModelID: id, Local: false})
	}
	return models, nil
}

type part struct {
	Text string `json:"text"`
}

type content struct {
	Parts []part `json:"parts"`
	Role  string `json:"role,omitempty"`
}

type generateRequest struct {
	Contents []content `json:"contents"`
}

type usageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
}

type candidate struct {
	Content content `json:"content"`
}

type generateResponse struct {
	Candidates    []candidate    `json:"candidates"`
	UsageMetadata *usageMetadata `json:"usageMetadata"`
}

func requestBody(req core.Request) generateRequest {
	return generateRequest{Contents: []content{{Parts: []part{{Text: req.Prompt}}}}}
}

func extractText(gr generateResponse) string {
	if len(gr.Candidates) == 0 || len(gr.Candidates[0].Content.Parts) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, p := range gr.Candidates[0].Content.Parts {
		sb.WriteString(p.Text)
	}
	return sb.String()
}

func extractUsage(gr generateResponse) core.Usage {
	if gr.UsageMetadata == nil {
		return core.Usage{}
	}
	return core.Usage{InputTokens: gr.UsageMetadata.PromptTokenCount, OutputTokens: gr.UsageMetadata.CandidatesTokenCount}
}

func (p *Provider) Complete(ctx context.Context, req core.Request) (core.Response, error) {
	body, err := json.Marshal(requestBody(req))
	if err != nil {
		return core.Response{}, fmt.Errorf("gemini: encode request: %w", err)
	}

	resp, err := p.do(ctx, http.MethodPost, "/models/"+req.ModelID+":generateContent", body)
	if err != nil {
		return core.Response{}, err
	}
	defer resp.Body.Close()
	if err := classifyStatus(resp); err != nil {
		return core.Response{}, err
	}

	var gr generateResponse
	if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil {
		return core.Response{}, fmt.Errorf("gemini: decode response: %w", err)
	}

	return core.Response{ModelID: req.ModelID, Content: extractText(gr), Usage: extractUsage(gr)}, nil
}

func (p *Provider) Stream(ctx context.Context, req core.Request) (<-chan core.Chunk, error) {
	body, err := json.Marshal(requestBody(req))
	if err != nil {
		return nil, fmt.Errorf("gemini: encode request: %w", err)
	}

	resp, err := p.do(ctx, http.MethodPost, "/models/"+req.ModelID+":streamGenerateContent?alt=sse", body)
	if err != nil {
		return nil, err
	}
	if err := classifyStatus(resp); err != nil {
		resp.Body.Close()
		return nil, err
	}

	ch := make(chan core.Chunk)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		var lastUsage *core.Usage
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			data, ok := strings.CutPrefix(line, "data: ")
			if !ok || data == "" {
				continue
			}
			var gr generateResponse
			if err := json.Unmarshal([]byte(data), &gr); err != nil {
				select {
				case ch <- core.Chunk{Err: fmt.Errorf("gemini: decode stream event: %w", err)}:
				case <-ctx.Done():
				}
				return
			}
			if gr.UsageMetadata != nil {
				u := extractUsage(gr)
				lastUsage = &u
			}
			select {
			case ch <- core.Chunk{Delta: extractText(gr)}:
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil {
			select {
			case ch <- core.Chunk{Err: fmt.Errorf("gemini: read stream: %w", err)}:
			case <-ctx.Done():
			}
			return
		}
		select {
		case ch <- core.Chunk{Done: true, Usage: lastUsage}:
		case <-ctx.Done():
		}
	}()
	return ch, nil
}

func (p *Provider) do(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	url := p.baseURL + path
	if strings.Contains(url, "?") {
		url += "&key=" + p.apiKey
	} else {
		url += "?key=" + p.apiKey
	}

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return nil, fmt.Errorf("gemini: build request: %w", err)
	}
	if body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	resp, err := p.http.Do(httpReq)
	if err != nil {
		// *url.Error.Error() incluye la URL completa -- con ?key=...
		// adentro. Redactar antes de que se convierta en ModelRun.Err
		// o log, nunca propagar tal cual (regla dura #4).
		return nil, fmt.Errorf("gemini: request failed: %s: %w", core.RedactSecrets(err.Error()), core.ErrRetryable)
	}
	return resp, nil
}

func classifyStatus(resp *http.Response) error {
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	msg := core.RedactSecrets(string(body))

	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return &core.RateLimitError{RetryAfter: retryAfter(resp)}
	case resp.StatusCode >= 500:
		return fmt.Errorf("gemini: %s: %s: %w", resp.Status, msg, core.ErrRetryable)
	default:
		return fmt.Errorf("gemini: %s: %s", resp.Status, msg)
	}
}

func retryAfter(resp *http.Response) (d time.Duration) {
	v := resp.Header.Get("Retry-After")
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil {
		return time.Duration(secs) * time.Second
	}
	return 0
}
