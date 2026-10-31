package strategy

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/allfitness/ai-gateway/internal/domain"
	"github.com/allfitness/ai-gateway/internal/openrouter"
	"github.com/allfitness/ai-gateway/internal/prompt"
)

// WorkoutStrategy handles GENERATE_WORKOUT requests.
type WorkoutStrategy struct {
	tm *prompt.TemplateManager
}

// NewWorkoutStrategy creates a new WorkoutStrategy.
func NewWorkoutStrategy(tm *prompt.TemplateManager) *WorkoutStrategy {
	return &WorkoutStrategy{tm: tm}
}

func (s *WorkoutStrategy) RequestType() domain.RequestType {
	return domain.RequestTypeWorkout
}

func (s *WorkoutStrategy) BuildMessages(task *domain.TaskPayload) ([]openrouter.ChatMessage, error) {
	sysPrompt, userPrompt, err := s.tm.Render("workout_v1.tmpl", task)
	if err != nil {
		return nil, fmt.Errorf("failed to render workout template: %w", err)
	}

	return []openrouter.ChatMessage{
		{Role: "system", Content: sysPrompt},
		{Role: "user", Content: userPrompt},
	}, nil
}

func (s *WorkoutStrategy) ValidateAndParse(raw []byte) (interface{}, error) {
	cleaned := CleanJSONResponse(string(raw))

	var resp domain.WorkoutResponse
	if err := json.Unmarshal(cleaned, &resp); err != nil {
		return nil, fmt.Errorf("json unmarshal failed: %w", err)
	}

	// Schema validations
	if strings.TrimSpace(resp.Name) == "" {
		return nil, fmt.Errorf("missing or empty 'name' field")
	}
	if resp.EstimatedDuration <= 0 {
		return nil, fmt.Errorf("'estimated_duration' must be a positive integer, got %d", resp.EstimatedDuration)
	}
	if len(resp.Exercises) == 0 {
		return nil, fmt.Errorf("'exercises' array must contain at least one exercise")
	}

	for i, ex := range resp.Exercises {
		if strings.TrimSpace(ex.ExerciseName) == "" {
			return nil, fmt.Errorf("exercise at index %d has empty 'exercise_name'", i)
		}
		if ex.Sets <= 0 {
			return nil, fmt.Errorf("exercise '%s' has invalid 'sets' count: %d", ex.ExerciseName, ex.Sets)
		}
		if strings.TrimSpace(ex.Reps) == "" {
			return nil, fmt.Errorf("exercise '%s' has empty 'reps' field", ex.ExerciseName)
		}
	}

	return resp, nil
}

func (s *WorkoutStrategy) SchemaDefinition() string {
	return `{
  "name": "string (required, non-empty)",
  "estimated_duration": "integer > 0 (required)",
  "exercises": [
    {
      "exercise_name": "string (required, non-empty)",
      "sets": "integer > 0 (required)",
      "reps": "string (required, e.g. '8-12')",
      "rest_seconds": "integer >= 0",
      "notes": "string (optional)"
    }
  ]
}`
}
