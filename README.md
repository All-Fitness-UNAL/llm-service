# All Fitness AI Harness / Gateway Service in Go

[![Go Version](https://img.shields.io/badge/go-1.22%2B-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![Docker](https://img.shields.io/badge/docker-ready-blue.svg)](Dockerfile)

A production-ready microservice written in Go that acts as an **AI Gateway and Harness** between the All Fitness core backend and the **OpenRouter API**.

The service consumes generation tasks asynchronously from a **Redis Queue**, executes prompt sanitization and prompt injection defense, dispatches generation calls to OpenRouter with automatic model fallback and retries, enforces strict schema validation with automatic repair retries, and persists results with TTL in Redis.

---

## Architecture Overview

```
                      ┌────────────────────────────────────────┐
                      │           Main Backend                 │
                      └──────────────────┬─────────────────────┘
                                         │ LPUSH Task
                                         ▼
                             ┌───────────────────────┐
                             │  Redis Task Queue     │
                             │ (allfitness:tasks:    │
                             │        queue)         │
                             └───────────┬───────────┘
                                         │ BRPOP (Worker Pool)
                                         ▼
┌──────────────────────────────────────────────────────────────────────────────────┐
│                             AI Gateway Microservice                              │
│                                                                                  │
│   ┌──────────────────────────────────────────────────────────────────────────┐   │
│   │                         Worker Pool Concurrency                          │   │
│   │                 (Configurable Goroutines: WORKER_COUNT=5)                │   │
│   └────────────────────────────────────┬─────────────────────────────────────┘   │
│                                        │                                         │
│   ┌────────────────────────────────────▼─────────────────────────────────────┐   │
│   │            1. Prompt Injection Defense & Privacy Sanitization            │   │
│   │  - System instruction override filtering ("ignore previous...", etc.)    │   │
│   │  - Delimiter & jailbreak detection (<|im_start|>, DAN, etc.)             │   │
│   │  - PII Scrubbing (Email, Phone, SSN, Credit Cards)                       │   │
│   └────────────────────────────────────┬─────────────────────────────────────┘   │
│                                        │                                         │
│   ┌────────────────────────────────────▼─────────────────────────────────────┐   │
│   │          2. Strategy Pattern Request Routing (text/template)             │   │
│   │  - GENERATE_WORKOUT  ==> workout_v1.tmpl                                 │   │
│   │  - GENERATE_RECIPE   ==> recipe_v1.tmpl                                  │   │
│   │  - GENERATE_EXERCISE ==> exercise_v1.tmpl                                │   │
│   └────────────────────────────────────┬─────────────────────────────────────┘   │
│                                        │                                         │
│   ┌────────────────────────────────────▼─────────────────────────────────────┐   │
│   │                     3. OpenRouter Client & Resilience                    │   │
│   │  - Primary Model (anthropic/claude-3.5-sonnet)                           │   │
│   │  - Exponential backoff with jitter on HTTP 429 & 5xx                     │   │
│   │  - Seamless Secondary Fallback Model (openai/gpt-4o-mini)                │   │
│   └────────────────────────────────────┬─────────────────────────────────────┘   │
│                                        │                                         │
│   ┌────────────────────────────────────▼─────────────────────────────────────┐   │
│   │                4. Strict Schema Enforcement & Auto-Repair                │   │
│   │  - Strict typed unmarshaling into DB entity structs                      │   │
│   │  - Up to 2 automatic LLM repair retries if invalid JSON/schema           │   │
│   └────────────────────────────────────┬─────────────────────────────────────┘   │
└────────────────────────────────────────┼─────────────────────────────────────────┘
                                         │ SET (15m TTL)
                                         ▼
                             ┌───────────────────────┐
                             │  Redis Result Store   │
                             │ (allfitness:results:  │
                             │     {request_id})     │
                             └───────────────────────┘
```

---

## Core Features

### 1. Redis Task Worker System
- Powered by `github.com/redis/go-redis/v9`.
- Non-blocking, context-aware worker pool running configurable goroutines (`WORKER_COUNT=5`).
- Continuous task ingestion via FIFO queue using `LPUSH` / `BRPOP`.
- Results saved to Redis under key `allfitness:results:{request_id}` with configurable expiration TTL (default `15m`).
- Graceful shutdown handling (`SIGINT`, `SIGTERM`), waiting for in-flight tasks to complete.

### 2. Request Routing & Polymorphic Handlers (Strategy Pattern)
- Modular `Strategy` interface handling prompt construction and schema validation for each request type:
  - `GENERATE_WORKOUT`: Generates tailored training programs matching user biometrics, goals, and equipment.
  - `GENERATE_RECIPE`: Generates macronutrient-accurate culinary recipes with ingredients and instructions.
  - `GENERATE_EXERCISE`: Generates targeted biomechanical exercise instructions and safety cues.

### 3. Prompt Engineering & Sanitization Layer
- **Prompt Injection Defense:** Fast regex and token-scanning guardrails against:
  - System prompt overrides (`ignore previous instructions`, `disregard directives`, etc.).
  - Jailbreak modes (`DAN mode`, `unrestricted AI`, `developer mode`).
  - Chat markup and delimiter attacks (`<|im_start|>`, `[INST]`, `<<SYS>>`).
  - Extraction attacks (`reveal system prompt`, `show hidden instructions`).
  - Automatically returns `PROMPT_INJECTION_DETECTED` error without calling the LLM.
- **Privacy Sanitization:** Automatically masks PII (Emails, Phones, SSNs, Credit Cards) before sending prompts to external APIs.
- **Template System:** Modular `text/template` files located in `./prompts/` with embedded Go fallback assets (`//go:embed`).

### 4. OpenRouter API Integration
- Direct HTTP POST calls to OpenRouter (`https://openrouter.ai/api/v1/chat/completions`).
- Strict JSON enforcement with `response_format: { "type": "json_object" }`.
- Configurable retry policy with exponential backoff and jitter on HTTP 429 and 5xx errors.
- Automatic model fallback from Primary model (`anthropic/claude-3.5-sonnet`) to Secondary model (`openai/gpt-4o-mini`).
- Built-in Mock Mode (`MOCK_OPENROUTER=true`) for local development and offline testing.

### 5. Strict Data Validation & Automatic Schema Repair
- Unmarshals and validates against strict Go domain structs matching database entities:
  - **Workout:** `name` (str), `estimated_duration` (int), `exercises` ([]WorkoutExercise).
  - **Recipe:** `title` (str), `description` (str), `ingredients` (str), `preparation_time` (int), `calories` (int), `protein` (float), `carbs` (float), `fats` (float), `instructions` ([]str).
  - **Exercise:** `name` (str), `category` (str), `muscle_group` (str), `difficulty` (str), `equipment_needed` (str), `instructions` ([]str), `tips` ([]str).
- If the LLM generates invalid JSON or missing required fields, the service executes **up to 2 automatic repair retries**, feeding the validation error back to the LLM to fix the output before failing.

---

## Directory Layout

```
.
├── cmd/
│   ├── worker/
│   │   └── main.go          # Microservice worker daemon entrypoint
│   └── producer/
│       └── main.go          # CLI utility to push test payloads and inspect results
├── internal/
│   ├── config/              # Environment variable loading and configuration
│   ├── domain/              # Core domain entities, DTOs, schemas, and error codes
│   ├── openrouter/          # OpenRouter client, retry policy, fallback, and mock
│   ├── prompt/              # Prompt templates, injection defense, and PII sanitizer
│   ├── queue/               # Redis queue consumer, publisher, and worker pool
│   └── strategy/            # Polymorphic strategy implementations (Workout, Recipe, Exercise)
├── prompts/                 # Modular .tmpl prompt templates
│   ├── workout_v1.tmpl
│   ├── recipe_v1.tmpl
│   └── exercise_v1.tmpl
├── Dockerfile               # Multi-stage minimal production container
├── docker-compose.yml       # Orchestrates Redis and the AI Gateway service
├── go.mod
├── go.sum
├── .env.example
└── README.md
```

---

## Configuration Variables

| Variable | Default Value | Description |
| :--- | :--- | :--- |
| `REDIS_ADDR` | `localhost:6379` | Host and port for Redis |
| `REDIS_PASSWORD` | `""` | Optional Redis password |
| `REDIS_DB` | `0` | Redis database index |
| `REDIS_TASK_QUEUE` | `allfitness:tasks:queue` | Redis key for task queue |
| `REDIS_RESULT_PREFIX` | `allfitness:results:` | Key prefix for stored results |
| `REDIS_RESULT_TTL` | `15m` | TTL for completed results |
| `WORKER_COUNT` | `5` | Number of concurrent worker goroutines |
| `MAX_REPAIR_RETRIES` | `2` | Maximum schema repair retries per task |
| `OPENROUTER_API_KEY` | `""` | OpenRouter API authentication token |
| `OPENROUTER_BASE_URL` | `https://openrouter.ai/api/v1` | OpenRouter API base URL |
| `OPENROUTER_PRIMARY_MODEL` | `anthropic/claude-3.5-sonnet` | Primary generation model |
| `OPENROUTER_FALLBACK_MODEL` | `openai/gpt-4o-mini` | Secondary model fallback |
| `OPENROUTER_MAX_RETRIES` | `3` | HTTP retries per model on 429/5xx |
| `OPENROUTER_TIMEOUT` | `60s` | HTTP request timeout |
| `PROMPTS_DIR` | `./prompts` | Path to directory containing `.tmpl` files |
| `APP_ENV` | `development` | Environment (`development` or `production`) |
| `LOG_LEVEL` | `INFO` | Logging level (`DEBUG`, `INFO`, `WARN`, `ERROR`) |
| `MOCK_OPENROUTER` | `false` | Enable mock mode (automatic if API key empty) |

---

## Getting Started

### Prerequisites
- Go 1.22+
- Redis 7+ (or Docker)
- OpenRouter API Key (optional for mock mode)

### 1. Running Locally with Go

1. Clone or navigate to the project directory:
   ```bash
   cd "LLM Service"
   ```

2. Copy the example configuration:
   ```bash
   cp .env.example .env
   ```

3. Ensure Redis is running locally:
   ```bash
   docker run -d -p 6379:6379 --name redis redis:7-alpine
   ```

4. Run the worker microservice:
   ```bash
   go run ./cmd/worker
   ```

### 2. Running with Docker Compose

Run the entire stack (Redis + Worker Microservice) in one command:

```bash
docker compose up --build
```

---

## Testing & Verification

### Running the Automated Test Suite

All modules have comprehensive unit and integration tests:

```bash
go test -v ./...
```

Tests verify:
- Prompt injection defense against multiple jailbreak vectors.
- PII scrubbing on email, phone, credit card, and SSN.
- Template rendering with parameter substitution.
- Strategy parsing and strict schema validations.
- OpenRouter retry on HTTP 429 (handling `Retry-After`).
- OpenRouter model fallback on primary 503 outage.
- Worker pool schema repair retries and Redis result envelope.

### Testing with the Producer CLI

A CLI utility is provided in `cmd/producer/main.go` to push tasks and observe real-time processing:

1. **Test Workout Generation:**
   ```bash
   go run ./cmd/producer -type workout
   ```

2. **Test Recipe Generation:**
   ```bash
   go run ./cmd/producer -type recipe
   ```

3. **Test Exercise Guide Generation:**
   ```bash
   go run ./cmd/producer -type exercise
   ```

4. **Test Prompt Injection Defense:**
   ```bash
   go run ./cmd/producer -type injection
   ```
   *Expected Output:* Status `FAILED` with Error Code `PROMPT_INJECTION_DETECTED`.

---

## Payload Specifications

### Input Task Payload (JSON read from Redis)

```json
{
  "request_id": "req-987654",
  "request_type": "GENERATE_WORKOUT",
  "created_at": "2026-10-30T19:45:00Z",
  "user_profile": {
    "user_id": "usr-456",
    "age": 28,
    "height": 180.0,
    "weight": 78.5,
    "experience_level": "intermediate",
    "goal": "Hypertrophy",
    "time_availability": 45,
    "diet_description": "High protein, balanced",
    "objective_description": "Chest and Triceps focus"
  },
  "available_equipment": ["Dumbbells", "Incline Bench"],
  "parameters": {
    "intensity": "high"
  }
}
```

### Result Payload (Stored under `allfitness:results:{request_id}`)

#### Success Example:
```json
{
  "request_id": "req-987654",
  "request_type": "GENERATE_WORKOUT",
  "status": "SUCCESS",
  "data": {
    "name": "Hypertrophy Upper Body Power & Pump",
    "estimated_duration": 45,
    "exercises": [
      {
        "exercise_name": "Incline Dumbbell Bench Press",
        "sets": 4,
        "reps": "8-12",
        "rest_seconds": 90,
        "notes": "Keep shoulder blades retracted and control the eccentric tempo."
      }
    ]
  },
  "execution_time_ms": 1420,
  "model_used": "anthropic/claude-3.5-sonnet",
  "repair_attempts": 0,
  "completed_at": "2026-10-30T19:45:02Z"
}
```

#### Failure (Injection Blocked) Example:
```json
{
  "request_id": "req-injection-test",
  "request_type": "GENERATE_WORKOUT",
  "status": "FAILED",
  "error_code": "PROMPT_INJECTION_DETECTED",
  "error_message": "Prompt injection detected: input matches injection rule in field 'diet_description'",
  "execution_time_ms": 1,
  "completed_at": "2026-10-30T19:45:01Z"
}
```

---

## Error Codes Reference

| Error Code | Trigger Condition |
| :--- | :--- |
| `PROMPT_INJECTION_DETECTED` | User input triggered jailbreak, instruction override, or delimiter rules. |
| `INVALID_PAYLOAD` | Missing required fields (`request_id`, empty body, malformed JSON). |
| `UNSUPPORTED_REQUEST_TYPE` | Unknown or missing `request_type`. |
| `LLM_API_ERROR` | All models (primary and fallback) failed after retries. |
| `SCHEMA_VALIDATION_FAILED` | Output violated schema and failed all repair retries. |
| `INTERNAL_ERROR` | Template execution or server serialization error. |

---

## License

All Fitness Microservices © 2026. Distributed under the MIT License.
