package openrouter

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/All-Fitness-UNAL/llm-service/internal/config"
)

func TestOpenRouterClient_SuccessPrimary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check headers
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Bearer test-key, got %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("HTTP-Referer") != "https://allfitness.com" {
			t.Errorf("expected referer, got %s", r.Header.Get("HTTP-Referer"))
		}

		resp := ChatCompletionResponse{
			ID:    "gen-123",
			Model: "anthropic/claude-3.5-sonnet",
			Choices: []Choice{
				{
					Index: 0,
					Message: ChatMessage{
						Role:    "assistant",
						Content: `{"name": "Leg Day"}`,
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := &config.Config{
		OpenRouterAPIKey:        "test-key",
		OpenRouterBaseURL:       server.URL,
		OpenRouterPrimaryModel:  "anthropic/claude-3.5-sonnet",
		OpenRouterFallbackModel: "openai/gpt-4o-mini",
		OpenRouterSiteURL:       "https://allfitness.com",
		OpenRouterAppName:       "All Fitness AI Gateway",
		OpenRouterMaxRetries:    2,
		OpenRouterTimeout:       5 * time.Second,
	}

	client := NewClient(cfg)
	res, model, err := client.GenerateWithFallback(context.Background(), []ChatMessage{
		{Role: "user", Content: "Give me a leg routine"},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model != "anthropic/claude-3.5-sonnet" {
		t.Errorf("expected primary model, got: %s", model)
	}
	if !strings.Contains(res, "Leg Day") {
		t.Errorf("expected response to contain 'Leg Day', got: %s", res)
	}
}

func TestOpenRouterClient_FallbackOnPrimaryFailure(t *testing.T) {
	var primaryCalls int32
	var fallbackCalls int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ChatCompletionRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		if req.Model == "anthropic/claude-3.5-sonnet" {
			atomic.AddInt32(&primaryCalls, 1)
			// Return 503 Service Unavailable for primary
			http.Error(w, `{"error": {"message": "Model overloaded"}}`, http.StatusServiceUnavailable)
			return
		}

		if req.Model == "openai/gpt-4o-mini" {
			atomic.AddInt32(&fallbackCalls, 1)
			resp := ChatCompletionResponse{
				ID:    "gen-456",
				Model: "openai/gpt-4o-mini",
				Choices: []Choice{
					{
						Index: 0,
						Message: ChatMessage{
							Role:    "assistant",
							Content: `{"name": "Fallback Routine"}`,
						},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		http.Error(w, "unknown model", http.StatusBadRequest)
	}))
	defer server.Close()

	cfg := &config.Config{
		OpenRouterAPIKey:        "test-key",
		OpenRouterBaseURL:       server.URL,
		OpenRouterPrimaryModel:  "anthropic/claude-3.5-sonnet",
		OpenRouterFallbackModel: "openai/gpt-4o-mini",
		OpenRouterMaxRetries:    2,
		OpenRouterTimeout:       5 * time.Second,
	}

	client := NewClient(cfg)
	res, model, err := client.GenerateWithFallback(context.Background(), []ChatMessage{
		{Role: "user", Content: "Build workout"},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model != "openai/gpt-4o-mini" {
		t.Errorf("expected fallback model, got: %s", model)
	}
	if !strings.Contains(res, "Fallback Routine") {
		t.Errorf("expected fallback response, got: %s", res)
	}
	if atomic.LoadInt32(&primaryCalls) < 1 {
		t.Errorf("expected primary model to be called, got 0")
	}
	if atomic.LoadInt32(&fallbackCalls) != 1 {
		t.Errorf("expected fallback model to be called once, got %d", atomic.LoadInt32(&fallbackCalls))
	}
}

func TestOpenRouterClient_RetryOn429(t *testing.T) {
	var calls int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&calls, 1)
		if count == 1 {
			// First call returns 429
			w.Header().Set("Retry-After", "1")
			http.Error(w, `{"error": "rate limit"}`, http.StatusTooManyRequests)
			return
		}

		// Second call succeeds
		resp := ChatCompletionResponse{
			ID:    "gen-789",
			Model: "anthropic/claude-3.5-sonnet",
			Choices: []Choice{
				{
					Message: ChatMessage{
						Role:    "assistant",
						Content: `{"name": "Rate Limit Recovered"}`,
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := &config.Config{
		OpenRouterAPIKey:        "test-key",
		OpenRouterBaseURL:       server.URL,
		OpenRouterPrimaryModel:  "anthropic/claude-3.5-sonnet",
		OpenRouterFallbackModel: "openai/gpt-4o-mini",
		OpenRouterMaxRetries:    3,
		OpenRouterTimeout:       5 * time.Second,
	}

	client := NewClient(cfg)
	res, _, err := client.GenerateWithFallback(context.Background(), []ChatMessage{
		{Role: "user", Content: "Test"},
	})

	if err != nil {
		t.Fatalf("expected retry recovery, got error: %v", err)
	}
	if !strings.Contains(res, "Rate Limit Recovered") {
		t.Errorf("unexpected content: %s", res)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Errorf("expected exactly 2 calls, got %d", atomic.LoadInt32(&calls))
	}
}

func TestMockClient_RepairJSON(t *testing.T) {
	mock := NewMockClient()
	repaired, model, err := mock.RepairJSON(
		context.Background(),
		"test-model",
		"{broken: true}",
		errors.New("invalid syntax"),
		"valid json schema",
	)

	if err != nil {
		t.Fatalf("unexpected mock repair error: %v", err)
	}
	if model == "" {
		t.Errorf("expected model name")
	}
	if !strings.Contains(repaired, "Repaired Upper Body Routine") {
		t.Errorf("unexpected repaired content: %s", repaired)
	}
}

