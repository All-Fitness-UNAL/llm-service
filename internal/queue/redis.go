package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/allfitness/ai-gateway/internal/config"
	"github.com/allfitness/ai-gateway/internal/domain"
	"github.com/redis/go-redis/v9"
)

// TaskQueue defines the operations required for consuming tasks and persisting results.
type TaskQueue interface {
	PopTask(ctx context.Context, timeout time.Duration) (*domain.TaskPayload, error)
	PushTask(ctx context.Context, task *domain.TaskPayload) error
	SaveResult(ctx context.Context, result *domain.TaskResult) error
	GetResult(ctx context.Context, requestID string) (*domain.TaskResult, error)
	Close() error
}

// RedisQueue implements TaskQueue using go-redis.
type RedisQueue struct {
	client       *redis.Client
	taskQueueKey string
	resultPrefix string
	resultTTL    time.Duration
}

// NewRedisQueue creates and tests a Redis client connection.
func NewRedisQueue(cfg *config.Config) (*RedisQueue, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis at %s: %w", cfg.RedisAddr, err)
	}

	return &RedisQueue{
		client:       rdb,
		taskQueueKey: cfg.RedisTaskQueue,
		resultPrefix: cfg.RedisResultPrefix,
		resultTTL:    cfg.RedisResultTTL,
	}, nil
}

// Close gracefully closes the Redis client connection.
func (q *RedisQueue) Close() error {
	return q.client.Close()
}

// PopTask blocks for up to timeout waiting for a task from the Redis queue.
func (q *RedisQueue) PopTask(ctx context.Context, timeout time.Duration) (*domain.TaskPayload, error) {
	res, err := q.client.BRPop(ctx, timeout, q.taskQueueKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return nil, nil
		}
		return nil, err
	}

	if len(res) < 2 {
		return nil, nil
	}

	var payload domain.TaskPayload
	if err := json.Unmarshal([]byte(res[1]), &payload); err != nil {
		return nil, fmt.Errorf("malformed task json: %w", err)
	}

	return &payload, nil
}

// PushTask pushes a task onto the task queue (LPUSH so BRPOP retrieves oldest first = FIFO).
func (q *RedisQueue) PushTask(ctx context.Context, task *domain.TaskPayload) error {
	data, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("failed to marshal task payload: %w", err)
	}
	return q.client.LPush(ctx, q.taskQueueKey, data).Err()
}

// SaveResult saves the final TaskResult to Redis under `allfitness:results:{request_id}` with TTL.
func (q *RedisQueue) SaveResult(ctx context.Context, result *domain.TaskResult) error {
	if result == nil || result.RequestID == "" {
		return fmt.Errorf("cannot save result with empty request_id")
	}

	key := fmt.Sprintf("%s%s", q.resultPrefix, result.RequestID)
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal task result: %w", err)
	}

	return q.client.Set(ctx, key, data, q.resultTTL).Err()
}

// GetResult retrieves the TaskResult from Redis by request_id.
func (q *RedisQueue) GetResult(ctx context.Context, requestID string) (*domain.TaskResult, error) {
	key := fmt.Sprintf("%s%s", q.resultPrefix, requestID)
	val, err := q.client.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	var res domain.TaskResult
	if err := json.Unmarshal([]byte(val), &res); err != nil {
		return nil, err
	}
	return &res, nil
}
