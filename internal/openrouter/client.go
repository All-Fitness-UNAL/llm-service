package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/All-Fitness-UNAL/llm-service/internal/config"
	"github.com/All-Fitness-UNAL/llm-service/internal/domain"
)

var (
	ErrAllModelsFailed = errors.New("all models (primary and fallback) failed to generate response")
	ErrRateLimited     = errors.New("openrouter rate limit exceeded")
)

// ChatMessage represents a single message in the LLM chat completion request.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ResponseFormat specifies the expected structure from LLM.
type ResponseFormat struct {
	Type string `json:"type"` // e.g. "json_object"
}

// ChatCompletionRequest is the payload sent to OpenRouter /chat/completions.
type ChatCompletionRequest struct {
	Model          string          `json:"model"`
	Messages       []ChatMessage   `json:"messages"`
	ResponseFormat *ResponseFormat `json:"response_format,omitempty"`
	Temperature    *float64        `json:"temperature,omitempty"`
}

// Choice represents an output option returned by OpenRouter.
type Choice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// APIError represents error details returned in OpenRouter response body.
type APIError struct {
	Message string      `json:"message"`
	Code    interface{} `json:"code,omitempty"`
}

// ChatCompletionResponse is the OpenRouter API response structure.
type ChatCompletionResponse struct {
	ID      string     `json:"id"`
	Model   string     `json:"model"`
	Choices []Choice   `json:"choices"`
	Error   *APIError  `json:"error,omitempty"`
}

// Client defines the interface for interacting with OpenRouter LLM completions.
type Client interface {
	GenerateWithFallback(ctx context.Context, messages []ChatMessage) (content string, modelUsed string, err error)
	RepairJSON(ctx context.Context, model string, rawOutput string, schemaError error, schemaInstructions string) (repairedContent string, modelUsed string, err error)
}

// OpenRouterClient implements Client with retry backoff and model fallbacks.
type OpenRouterClient struct {
	cfg        *config.Config
	httpClient *http.Client
	rng        *rand.Rand
}

// NewClient creates a new OpenRouter client.
func NewClient(cfg *config.Config) Client {
	if cfg.MockOpenRouter {
		return NewMockClient()
	}

	return &OpenRouterClient{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: cfg.OpenRouterTimeout,
		},
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// GenerateWithFallback executes the chat completion attempting the primary model, then falling back if it fails.
func (c *OpenRouterClient) GenerateWithFallback(ctx context.Context, messages []ChatMessage) (string, string, error) {
	temp := 0.3
	reqBody := ChatCompletionRequest{
		Messages: messages,
		ResponseFormat: &ResponseFormat{
			Type: "json_object",
		},
		Temperature: &temp,
	}

	// 1. Try Primary Model
	reqBody.Model = c.cfg.OpenRouterPrimaryModel
	slog.Info("calling primary model", "model", reqBody.Model)
	content, err := c.executeWithRetry(ctx, reqBody)
	if err == nil {
		return content, c.cfg.OpenRouterPrimaryModel, nil
	}

	slog.Warn("primary model failed, falling back to secondary model",
		"primary_model", c.cfg.OpenRouterPrimaryModel,
		"fallback_model", c.cfg.OpenRouterFallbackModel,
		"error", err,
	)

	// 2. Try Fallback Model
	reqBody.Model = c.cfg.OpenRouterFallbackModel
	content, errFallback := c.executeWithRetry(ctx, reqBody)
	if errFallback == nil {
		return content, c.cfg.OpenRouterFallbackModel, nil
	}

	return "", "", fmt.Errorf("%w: primary error: %v, fallback error: %v", ErrAllModelsFailed, err, errFallback)
}

// RepairJSON prompts the model to repair invalid JSON matching the strict schema.
func (c *OpenRouterClient) RepairJSON(ctx context.Context, model string, rawOutput string, schemaError error, schemaInstructions string) (string, string, error) {
	if model == "" {
		model = c.cfg.OpenRouterPrimaryModel
	}

	repairPrompt := fmt.Sprintf(
		"CRITICAL: The previous response produced invalid JSON or failed schema validation.\n"+
			"Validation Error: %s\n"+
			"Expected Schema:\n%s\n\n"+
			"Previous invalid response was:\n%s\n\n"+
			"Please fix and output ONLY the valid JSON object adhering strictly to the schema.",
		schemaError.Error(), schemaInstructions, rawOutput,
	)

	messages := []ChatMessage{
		{
			Role:    "system",
			Content: "You are a JSON repair and validation specialist. Return ONLY a single valid JSON object that strictly satisfies the requested schema without markdown or text explanations.",
		},
		{
			Role:    "user",
			Content: repairPrompt,
		},
	}

	temp := 0.1
	reqBody := ChatCompletionRequest{
		Model:    model,
		Messages: messages,
		ResponseFormat: &ResponseFormat{
			Type: "json_object",
		},
		Temperature: &temp,
	}

	slog.Info("requesting JSON repair from LLM", "model", model)
	content, err := c.executeWithRetry(ctx, reqBody)
	if err != nil {
		return "", model, fmt.Errorf("repair failed: %w", err)
	}

	return content, model, nil
}

// executeWithRetry executes an HTTP request to OpenRouter with exponential backoff on retryable status codes.
func (c *OpenRouterClient) executeWithRetry(ctx context.Context, reqPayload ChatCompletionRequest) (string, error) {
	var lastErr error
	maxRetries := c.cfg.OpenRouterMaxRetries
	if maxRetries < 1 {
		maxRetries = 1
	}

	url := fmt.Sprintf("%s/chat/completions", c.cfg.OpenRouterBaseURL)
	jsonData, err := json.Marshal(reqPayload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	for attempt := 0; attempt < maxRetries; attempt++ {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonData))
		if err != nil {
			return "", fmt.Errorf("failed to create http request: %w", err)
		}

		// Required & Recommended OpenRouter Headers
		httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.cfg.OpenRouterAPIKey))
		httpReq.Header.Set("Content-Type", "application/json")
		if c.cfg.OpenRouterSiteURL != "" {
			httpReq.Header.Set("HTTP-Referer", c.cfg.OpenRouterSiteURL)
		}
		if c.cfg.OpenRouterAppName != "" {
			httpReq.Header.Set("X-Title", c.cfg.OpenRouterAppName)
		}

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			lastErr = fmt.Errorf("http request error: %w", err)
			slog.Warn("openrouter request failed, will retry", "attempt", attempt+1, "error", lastErr)
			c.sleepWithJitter(ctx, attempt, 0)
			continue
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if err != nil {
			lastErr = fmt.Errorf("failed to read response body: %w", err)
			c.sleepWithJitter(ctx, attempt, 0)
			continue
		}

		// Handle Success
		if resp.StatusCode == http.StatusOK {
			var compResp ChatCompletionResponse
			if err := json.Unmarshal(bodyBytes, &compResp); err != nil {
				return "", fmt.Errorf("failed to parse openrouter response json: %w", err)
			}
			if len(compResp.Choices) == 0 {
				return "", fmt.Errorf("openrouter returned no choices")
			}
			return strings.TrimSpace(compResp.Choices[0].Message.Content), nil
		}

		// Handle Retryable Conditions: 429 Too Many Requests, 500, 502, 503, 504
		retryDelay := c.extractRetryAfter(resp)
		lastErr = fmt.Errorf("openrouter api returned HTTP %d: %s", resp.StatusCode, string(bodyBytes))

		if resp.StatusCode == http.StatusTooManyRequests || (resp.StatusCode >= 500 && resp.StatusCode <= 504) {
			slog.Warn("retryable error from openrouter",
				"status", resp.StatusCode,
				"attempt", attempt+1,
				"retry_delay_sec", retryDelay.Seconds(),
			)
			c.sleepWithJitter(ctx, attempt, retryDelay)
			continue
		}

		// Non-retryable error (e.g. 400 Bad Request, 401 Unauthorized, 403 Forbidden)
		return "", fmt.Errorf("%w: %s", domain.ErrPromptInjectionDetected, lastErr.Error())
	}

	return "", fmt.Errorf("max retries exceeded: %w", lastErr)
}

// extractRetryAfter checks the Retry-After header.
func (c *OpenRouterClient) extractRetryAfter(resp *http.Response) time.Duration {
	if h := resp.Header.Get("Retry-After"); h != "" {
		if seconds, err := strconv.Atoi(h); err == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
	}
	return 0
}

// sleepWithJitter calculates exponential backoff: base * 2^attempt + jitter.
func (c *OpenRouterClient) sleepWithJitter(ctx context.Context, attempt int, overrideDelay time.Duration) {
	if overrideDelay > 0 {
		select {
		case <-ctx.Done():
		case <-time.After(overrideDelay):
		}
		return
	}

	base := 1 * time.Second
	factor := 1 << attempt // 1, 2, 4, 8...
	backoff := base * time.Duration(factor)
	jitter := time.Duration(c.rng.Int63n(int64(500 * time.Millisecond)))
	total := backoff + jitter

	select {
	case <-ctx.Done():
	case <-time.After(total):
	}
}

