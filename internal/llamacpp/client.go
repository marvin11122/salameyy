package llamacpp

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"charm.land/catwalk/pkg/catwalk"
)

const (
	DefaultBaseURL   = "http://127.0.0.1:8080/v1/"
	maxModelsPayload = 4 << 20
)

type ModelInfo struct {
	ID            string
	Name          string
	ContextWindow int64
}

var billionSizePattern = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)b`)

func NormalizeBaseURL(baseURL, host string, port int) string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		host = strings.TrimSpace(host)
		if host == "" {
			host = "127.0.0.1"
		}
		if port <= 0 || port > 65535 {
			if hostOnly, hostPort, splitErr := net.SplitHostPort(strings.Trim(host, "[]")); splitErr == nil && hostPort != "" {
				baseURL = "http://" + net.JoinHostPort(hostOnly, hostPort)
			} else {
				port = 8080
				baseURL = "http://" + net.JoinHostPort(strings.Trim(host, "[]"), strconv.Itoa(port))
			}
		} else {
			baseURL = "http://" + net.JoinHostPort(strings.Trim(host, "[]"), strconv.Itoa(port))
		}
	}
	if !strings.Contains(baseURL, "://") {
		baseURL = "http://" + baseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return DefaultBaseURL
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return DefaultBaseURL
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if !strings.HasSuffix(parsed.Path, "/v1") {
		parsed.Path += "/v1"
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String() + "/"
}

func QueryModels(ctx context.Context, baseURL string) ([]ModelInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	baseURL = NormalizeBaseURL(baseURL, "", 0)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("build llama.cpp request: %w", err)
	}
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("query llama.cpp models: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("llama.cpp model endpoint returned %s", resp.Status)
	}
	var payload struct {
		Data []struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			ContextWindow int64  `json:"context_window"`
			ContextLength int64  `json:"context_length"`
			Context       int64  `json:"n_ctx_train"`
			Meta          struct {
				ContextWindow int64 `json:"n_ctx_train"`
				Context       int64 `json:"n_ctx"`
			} `json:"meta"`
		} `json:"data"`
		Models []struct {
			ID            string `json:"id"`
			Model         string `json:"model"`
			Name          string `json:"name"`
			ContextWindow int64  `json:"context_window"`
			ContextLength int64  `json:"context_length"`
			Context       int64  `json:"n_ctx_train"`
			Meta          struct {
				ContextWindow int64 `json:"n_ctx_train"`
				Context       int64 `json:"n_ctx"`
			} `json:"meta"`
		} `json:"models"`
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxModelsPayload+1))
	if err != nil {
		return nil, fmt.Errorf("read llama.cpp models: %w", err)
	}
	if len(body) > maxModelsPayload {
		return nil, fmt.Errorf("llama.cpp model response is too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode llama.cpp models: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("decode llama.cpp models: multiple JSON values")
		}
		return nil, fmt.Errorf("decode llama.cpp models: %w", err)
	}
	models := make([]ModelInfo, 0, max(len(payload.Data), len(payload.Models)))
	seen := make(map[string]struct{})
	add := func(id, name string, contexts ...int64) {
		id = strings.TrimSpace(id)
		if id == "" || len(id) > 1024 || strings.ContainsAny(id, "\x00\r\n") {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		contextWindow := int64(0)
		for _, candidate := range contexts {
			if candidate > 0 {
				contextWindow = candidate
				break
			}
		}
		if contextWindow <= 0 {
			contextWindow = 32768
		}
		models = append(models, ModelInfo{ID: id, Name: cmp.Or(strings.TrimSpace(name), id), ContextWindow: contextWindow})
	}
	for _, model := range payload.Data {
		add(model.ID, model.Name, model.Meta.ContextWindow, model.Meta.Context, model.ContextWindow, model.ContextLength, model.Context)
	}
	for _, model := range payload.Models {
		add(cmp.Or(model.Model, model.Name, model.ID), model.Name, model.Meta.ContextWindow, model.Meta.Context, model.ContextWindow, model.ContextLength, model.Context)
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("llama.cpp returned no models")
	}
	return models, nil
}

func WaitUntilReady(ctx context.Context, baseURL string, interval time.Duration) ([]ModelInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if interval <= 0 {
		interval = 250 * time.Millisecond
	}
	var lastErr error
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		models, err := QueryModels(ctx, baseURL)
		if err == nil {
			return models, nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, lastErr
		case <-ticker.C:
		}
	}
}

func ProviderFromURL(ctx context.Context, baseURL string) (catwalk.Provider, error) {
	baseURL = NormalizeBaseURL(baseURL, "", 0)
	models, err := QueryModels(ctx, baseURL)
	if err != nil {
		return catwalk.Provider{}, err
	}
	providerModels := make([]catwalk.Model, 0, len(models))
	for _, model := range models {
		contextWindow := model.ContextWindow
		if contextWindow <= 0 {
			contextWindow = 32768
		}
		providerModels = append(providerModels, catwalk.Model{ID: model.ID, Name: cmp.Or(model.Name, model.ID), ContextWindow: contextWindow, DefaultMaxTokens: min(int64(8192), contextWindow)})
	}
	large, small := SelectDefaultModels(models)
	return catwalk.Provider{Name: "llama.cpp", ID: catwalk.InferenceProvider("llamacpp"), APIEndpoint: baseURL, Type: catwalk.TypeOpenAICompat, DefaultLargeModelID: large.ID, DefaultSmallModelID: small.ID, Models: providerModels}, nil
}

func SelectDefaultModels(models []ModelInfo) (ModelInfo, ModelInfo) {
	if len(models) == 0 {
		return ModelInfo{}, ModelInfo{}
	}
	if len(models) == 1 {
		return models[0], models[0]
	}
	bestLarge, bestSmall := models[0], models[0]
	bestLargeScore, bestSmallScore := scoreLocalModel(models[0], false), scoreLocalModel(models[0], true)
	for _, model := range models[1:] {
		if score := scoreLocalModel(model, false); score > bestLargeScore {
			bestLarge, bestLargeScore = model, score
		}
		if score := scoreLocalModel(model, true); score > bestSmallScore {
			bestSmall, bestSmallScore = model, score
		}
	}
	return bestLarge, bestSmall
}

func scoreLocalModel(model ModelInfo, preferSmall bool) int {
	text := strings.ToLower(strings.TrimSpace(model.ID + " " + model.Name))
	contextWindow := model.ContextWindow
	if contextWindow <= 0 {
		contextWindow = 32768
	}
	coding := 90 * countKeywords(text, "coder", "codestral", "starcoder", "codellama", "devstral")
	coding += 70 * countKeywords(text, "code", "program", "developer")
	coding += 55 * countKeywords(text, "qwen", "deepseek", "gpt-oss", "oss")
	coding += 30 * countKeywords(text, "instruct", "assistant", "chat")
	coding += 16 * countKeywords(text, "q4", "iq4", "f16", "bf16")
	coding -= 180 * countKeywords(text, "embed", "embedding", "rerank")
	coding -= 120 * countKeywords(text, "vision", "vl", "image", "audio", "speech", "whisper")
	coding -= 90 * countKeywords(text, "guard", "safety", "moderation")
	size := parseBillionSize(text)
	sizeScore := 0.0
	if size > 0 {
		if preferSmall {
			target := 4.0
			if strings.Contains(text, "mini") {
				target = 3.0
			}
			sizeScore = 180.0 - math.Abs(size-target)*42.0
			if size > 12 {
				sizeScore -= (size - 12) * 28.0
			}
		} else {
			sizeScore = math.Min(size, 34.0) * 18.0
			if size > 48 {
				sizeScore -= (size - 48) * 8.0
			}
		}
	}
	contextScore := math.Min(float64(contextWindow), 131072.0) / 2048.0
	if preferSmall {
		contextScore = math.Min(float64(contextWindow), 65536.0) / 4096.0
	}
	score := float64(coding)*12.0 + sizeScore + contextScore
	if preferSmall && strings.Contains(text, "mini") {
		score += 25
	}
	if !preferSmall && strings.Contains(text, "thinking") {
		score += 18
	}
	if math.IsNaN(score) || math.IsInf(score, 0) {
		return -1 << 30
	}
	return int(math.Round(score))
}

func countKeywords(text string, keywords ...string) int {
	total := 0
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			total++
		}
	}
	return total
}

func parseBillionSize(text string) float64 {
	match := billionSizePattern.FindStringSubmatch(text)
	if len(match) != 2 {
		return 0
	}
	value, err := strconv.ParseFloat(match[1], 64)
	if err != nil || value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return value
}
