package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/All-Fitness-UNAL/llm-service/internal/config"
	"github.com/All-Fitness-UNAL/llm-service/internal/domain"
	"github.com/All-Fitness-UNAL/llm-service/internal/openrouter"
	"github.com/All-Fitness-UNAL/llm-service/internal/prompt"
	"github.com/All-Fitness-UNAL/llm-service/internal/strategy"
)

// WorkerPool manages concurrent worker goroutines that ingest and process tasks.
type WorkerPool struct {
	cfg        *config.Config
	queue      TaskQueue
	strategies *strategy.Registry
	sanitizer  *prompt.Sanitizer
	llmClient  openrouter.Client
	wg         sync.WaitGroup
	stopChan   chan struct{}
}

// NewWorkerPool initializes the worker pool.
func NewWorkerPool(
	cfg *config.Config,
	q TaskQueue,
	stratReg *strategy.Registry,
	sanitizer *prompt.Sanitizer,
	llmClient openrouter.Client,
) *WorkerPool {
	return &WorkerPool{
		cfg:        cfg,
		queue:      q,
		strategies: stratReg,
		sanitizer:  sanitizer,
		llmClient:  llmClient,
		stopChan:   make(chan struct{}),
	}
}

// Start launches the configured number of worker goroutines.
func (p *WorkerPool) Start(ctx context.Context) {
	slog.Info("starting worker pool", "worker_count", p.cfg.WorkerCount)

	for i := 1; i <= p.cfg.WorkerCount; i++ {
		p.wg.Add(1)
		go p.workerLoop(ctx, i)
	}
}

// Stop initiates graceful shutdown, waiting for active workers to drain.
func (p *WorkerPool) Stop() {
	slog.Info("stopping worker pool, waiting for in-flight tasks to complete...")
	close(p.stopChan)
	p.wg.Wait()
	slog.Info("worker pool gracefully stopped")
}

// workerLoop continuously polls the Redis queue for tasks until stopped or context cancelled.
func (p *WorkerPool) workerLoop(ctx context.Context, workerID int) {
	defer p.wg.Done()
	slog.Debug("worker started", "worker_id", workerID)

	for {
		select {
		case <-ctx.Done():
			slog.Debug("worker context cancelled", "worker_id", workerID)
			return
		case <-p.stopChan:
			slog.Debug("worker received stop signal", "worker_id", workerID)
			return
		default:
			// Poll with 2-second timeout to allow quick exit on shutdown
			task, err := p.queue.PopTask(ctx, 2*time.Second)
			if err != nil {
				slog.Error("error popping task from queue", "worker_id", workerID, "error", err)
				time.Sleep(1 * time.Second)
				continue
			}

			if task == nil {
				// Queue empty or timeout expired, loop again
				continue
			}

			slog.Info("task received",
				"worker_id", workerID,
				"request_id", task.RequestID,
				"request_type", task.RequestType,
			)

			p.processTask(ctx, task)
		}
	}
}

// processTask handles the lifecycle of a single task from validation to LLM generation and result persistence.
func (p *WorkerPool) processTask(ctx context.Context, task *domain.TaskPayload) {
	startTime := time.Now()
	result := &domain.TaskResult{
		RequestID:   task.RequestID,
		RequestType: task.RequestType,
		Status:      domain.StatusFailed,
		CompletedAt: time.Now().UTC(),
	}

	// 1. Basic Payload Validation
	if err := task.Validate(); err != nil {
		slog.Warn("invalid task payload", "request_id", task.RequestID, "error", err)
		result.ErrorCode = domain.ErrCodeInvalidPayload
		if errors.Is(err, domain.ErrUnsupportedRequestType) {
			result.ErrorCode = domain.ErrCodeUnsupportedType
		}
		result.ErrorMessage = err.Error()
		result.ExecutionTimeMs = time.Since(startTime).Milliseconds()
		p.persistResult(ctx, result)
		return
	}

	// 2. Prompt Injection Defense & Privacy Sanitization
	if err := p.sanitizer.SanitizeUserProfile(&task.UserProfile); err != nil {
		slog.Warn("security alert: prompt injection detected",
			"request_id", task.RequestID,
			"user_id", task.UserProfile.UserID,
			"error", err,
		)
		result.ErrorCode = domain.ErrCodePromptInjection
		result.ErrorMessage = fmt.Sprintf("Prompt injection detected: %v", err)
		result.ExecutionTimeMs = time.Since(startTime).Milliseconds()
		p.persistResult(ctx, result)
		return
	}

	// 3. Resolve Request Strategy
	strat, err := p.strategies.Get(task.RequestType)
	if err != nil {
		slog.Error("unsupported strategy", "request_id", task.RequestID, "request_type", task.RequestType)
		result.ErrorCode = domain.ErrCodeUnsupportedType
		result.ErrorMessage = err.Error()
		result.ExecutionTimeMs = time.Since(startTime).Milliseconds()
		p.persistResult(ctx, result)
		return
	}

	// 4. Build Prompt Messages from Templates
	messages, err := strat.BuildMessages(task)
	if err != nil {
		slog.Error("failed to construct prompt messages", "request_id", task.RequestID, "error", err)
		result.ErrorCode = domain.ErrCodeInternalError
		result.ErrorMessage = fmt.Sprintf("Prompt compilation failed: %v", err)
		result.ExecutionTimeMs = time.Since(startTime).Milliseconds()
		p.persistResult(ctx, result)
		return
	}

	// 5. OpenRouter LLM Invocation with Fallback Model Support
	rawOutput, modelUsed, err := p.llmClient.GenerateWithFallback(ctx, messages)
	result.ModelUsed = modelUsed

	if err != nil {
		slog.Error("openrouter generation failed", "request_id", task.RequestID, "error", err)
		result.ErrorCode = domain.ErrCodeLLMError
		result.ErrorMessage = fmt.Sprintf("LLM completion failed: %v", err)
		result.ExecutionTimeMs = time.Since(startTime).Milliseconds()
		p.persistResult(ctx, result)
		return
	}

	// 6. Strict Data Validation (Schema Enforcement) with Automatic Repair Retries
	parsedData, valErr := strat.ValidateAndParse([]byte(rawOutput))
	repairCount := 0

	for valErr != nil && repairCount < p.cfg.MaxRepairRetries {
		repairCount++
		slog.Warn("schema validation failed, triggering repair retry",
			"request_id", task.RequestID,
			"attempt", repairCount,
			"max_repairs", p.cfg.MaxRepairRetries,
			"error", valErr,
		)

		repairedOutput, repModel, repErr := p.llmClient.RepairJSON(
			ctx,
			modelUsed,
			rawOutput,
			valErr,
			strat.SchemaDefinition(),
		)
		if repErr != nil {
			slog.Error("repair request failed", "request_id", task.RequestID, "repair_attempt", repairCount, "error", repErr)
			break
		}

		result.ModelUsed = repModel
		rawOutput = repairedOutput
		parsedData, valErr = strat.ValidateAndParse([]byte(rawOutput))
	}

	result.RepairAttempts = repairCount

	// If schema validation still failed after repair retries
	if valErr != nil {
		slog.Error("strict schema validation permanently failed after repair retries",
			"request_id", task.RequestID,
			"error", valErr,
			"raw_output", rawOutput,
		)
		result.ErrorCode = domain.ErrCodeSchemaValidationFail
		result.ErrorMessage = fmt.Sprintf("Schema validation failed: %v", valErr)
		result.ExecutionTimeMs = time.Since(startTime).Milliseconds()
		p.persistResult(ctx, result)
		return
	}

	// 7. Successful Completion: Marshal Typed Output into Result
	dataBytes, err := json.Marshal(parsedData)
	if err != nil {
		result.ErrorCode = domain.ErrCodeInternalError
		result.ErrorMessage = fmt.Sprintf("Failed to marshal output data: %v", err)
		result.ExecutionTimeMs = time.Since(startTime).Milliseconds()
		p.persistResult(ctx, result)
		return
	}

	result.Status = domain.StatusSuccess
	result.Data = dataBytes
	result.ExecutionTimeMs = time.Since(startTime).Milliseconds()
	result.CompletedAt = time.Now().UTC()

	slog.Info("task completed successfully",
		"request_id", task.RequestID,
		"request_type", task.RequestType,
		"model_used", result.ModelUsed,
		"duration_ms", result.ExecutionTimeMs,
		"repair_attempts", result.RepairAttempts,
	)

	p.persistResult(ctx, result)
}

// persistResult writes the result to Redis with error logging.
func (p *WorkerPool) persistResult(ctx context.Context, result *domain.TaskResult) {
	if err := p.queue.SaveResult(ctx, result); err != nil {
		slog.Error("failed to persist result to redis", "request_id", result.RequestID, "error", err)
	}
}

