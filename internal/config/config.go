package config

import (
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all configuration parameters for the AI Gateway worker service.
type Config struct {
	// Redis configuration
	RedisAddr         string
	RedisPassword     string
	RedisDB           int
	RedisTaskQueue    string
	RedisResultPrefix string
	RedisResultTTL    time.Duration

	// Worker configuration
	WorkerCount      int
	ShutdownTimeout  time.Duration
	MaxRepairRetries int

	// OpenRouter configuration
	OpenRouterAPIKey        string
	OpenRouterBaseURL       string
	OpenRouterPrimaryModel  string
	OpenRouterFallbackModel string
	OpenRouterSiteURL       string
	OpenRouterAppName       string
	OpenRouterMaxRetries    int
	OpenRouterTimeout       time.Duration

	// Prompts configuration
	PromptsDir string

	// Environment & Logging
	AppEnv   string
	LogLevel slog.Level

	// Mock mode for development or integration tests when no API key is available
	MockOpenRouter bool
}

// Load reads configuration from environment variables and optionally from a .env file.
func Load() (*Config, error) {
	// Attempt to load .env file; ignore if missing
	_ = godotenv.Load()

	cfg := &Config{
		RedisAddr:               getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:           getEnv("REDIS_PASSWORD", ""),
		RedisDB:                 getEnvAsInt("REDIS_DB", 0),
		RedisTaskQueue:          getEnv("REDIS_TASK_QUEUE", "allfitness:tasks:queue"),
		RedisResultPrefix:       getEnv("REDIS_RESULT_PREFIX", "allfitness:results:"),
		RedisResultTTL:          getEnvAsDuration("REDIS_RESULT_TTL", 15*time.Minute),
		WorkerCount:             getEnvAsInt("WORKER_COUNT", 5),
		ShutdownTimeout:         getEnvAsDuration("SHUTDOWN_TIMEOUT", 15*time.Second),
		MaxRepairRetries:        getEnvAsInt("MAX_REPAIR_RETRIES", 2),
		OpenRouterAPIKey:        getEnv("OPENROUTER_API_KEY", ""),
		OpenRouterBaseURL:       strings.TrimRight(getEnv("OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1"), "/"),
		OpenRouterPrimaryModel:  getEnv("OPENROUTER_PRIMARY_MODEL", "xiaomi/mimo-v2.6-flash"),
		OpenRouterFallbackModel: getEnv("OPENROUTER_FALLBACK_MODEL", "nvidia/nemotron-3-ultra-550b-a55b:free"),
		OpenRouterSiteURL:       getEnv("OPENROUTER_SITE_URL", "https://allfitness.com"),
		OpenRouterAppName:       getEnv("OPENROUTER_APP_NAME", "All Fitness AI Gateway"),
		OpenRouterMaxRetries:    getEnvAsInt("OPENROUTER_MAX_RETRIES", 3),
		OpenRouterTimeout:       getEnvAsDuration("OPENROUTER_TIMEOUT", 60*time.Second),
		PromptsDir:              getEnv("PROMPTS_DIR", "./prompts"),
		AppEnv:                  getEnv("APP_ENV", "development"),
		LogLevel:                parseLogLevel(getEnv("LOG_LEVEL", "INFO")),
		MockOpenRouter:          getEnvAsBool("MOCK_OPENROUTER", false),
	}

	// If API key is empty and MOCK_OPENROUTER is not explicitly false, allow mock mode if flag set or warn
	if cfg.OpenRouterAPIKey == "" && !cfg.MockOpenRouter {
		slog.Warn("OPENROUTER_API_KEY is not set. Service will run in mock mode for LLM generation unless configured.")
		cfg.MockOpenRouter = true
	}

	return cfg, nil
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok && strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	return defaultVal
}

func getEnvAsInt(key string, defaultVal int) int {
	valStr := getEnv(key, "")
	if val, err := strconv.Atoi(valStr); err == nil {
		return val
	}
	return defaultVal
}

func getEnvAsBool(key string, defaultVal bool) bool {
	valStr := getEnv(key, "")
	if val, err := strconv.ParseBool(valStr); err == nil {
		return val
	}
	return defaultVal
}

func getEnvAsDuration(key string, defaultVal time.Duration) time.Duration {
	valStr := getEnv(key, "")
	if val, err := time.ParseDuration(valStr); err == nil {
		return val
	}
	return defaultVal
}

func parseLogLevel(level string) slog.Level {
	switch strings.ToUpper(level) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
