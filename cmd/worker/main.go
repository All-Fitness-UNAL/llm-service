package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/All-Fitness-UNAL/llm-service/internal/config"
	"github.com/All-Fitness-UNAL/llm-service/internal/openrouter"
	"github.com/All-Fitness-UNAL/llm-service/internal/prompt"
	"github.com/All-Fitness-UNAL/llm-service/internal/queue"
	"github.com/All-Fitness-UNAL/llm-service/internal/strategy"
)

func main() {
	// 1. Load Configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	// 2. Setup Structured Logging (slog)
	setupLogger(cfg)

	slog.Info("initializing All Fitness AI Gateway service...",
		"env", cfg.AppEnv,
		"workers", cfg.WorkerCount,
		"primary_model", cfg.OpenRouterPrimaryModel,
		"fallback_model", cfg.OpenRouterFallbackModel,
		"task_queue", cfg.RedisTaskQueue,
		"result_prefix", cfg.RedisResultPrefix,
		"mock_mode", cfg.MockOpenRouter,
	)

	// 3. Initialize Prompt Template Manager
	tm, err := prompt.NewTemplateManager(cfg.PromptsDir)
	if err != nil {
		slog.Error("failed to initialize prompt template manager", "error", err)
		os.Exit(1)
	}

	// 4. Initialize Sanitizer and Strategy Registry
	sanitizer := prompt.NewSanitizer()
	stratRegistry := strategy.NewRegistry(tm)

	// 5. Initialize OpenRouter Client
	llmClient := openrouter.NewClient(cfg)

	// 6. Connect to Redis Queue
	redisQueue, err := queue.NewRedisQueue(cfg)
	if err != nil {
		slog.Error("failed to connect to Redis", "addr", cfg.RedisAddr, "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := redisQueue.Close(); err != nil {
			slog.Error("error closing redis connection", "error", err)
		}
	}()

	slog.Info("connected to Redis successfully", "addr", cfg.RedisAddr)

	// 7. Initialize and Start Worker Pool
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	workerPool := queue.NewWorkerPool(cfg, redisQueue, stratRegistry, sanitizer, llmClient)
	workerPool.Start(ctx)

	slog.Info("All Fitness AI Gateway is active and listening for tasks")

	// 8. Graceful Shutdown Signal Handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	sig := <-sigChan
	slog.Info("shutdown signal received, stopping service...", "signal", sig.String())

	// Cancel worker context and wait for active jobs to drain
	cancel()
	workerPool.Stop()

	slog.Info("All Fitness AI Gateway shut down successfully")
}

func setupLogger(cfg *config.Config) {
	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}

	if cfg.AppEnv == "production" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	slog.SetDefault(slog.New(handler))
}

