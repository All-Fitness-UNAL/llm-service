package strategy

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/All-Fitness-UNAL/llm-service/internal/domain"
	"github.com/All-Fitness-UNAL/llm-service/internal/openrouter"
	"github.com/All-Fitness-UNAL/llm-service/internal/prompt"
)

// ExerciseStrategy handles GENERATE_EXERCISE requests.
type ExerciseStrategy struct {
	tm *prompt.TemplateManager
}

// NewExerciseStrategy creates a new ExerciseStrategy.
func NewExerciseStrategy(tm *prompt.TemplateManager) *ExerciseStrategy {
	return &ExerciseStrategy{tm: tm}
}

func (s *ExerciseStrategy) RequestType() domain.RequestType {
	return domain.RequestTypeExercise
}

func (s *ExerciseStrategy) BuildMessages(task *domain.TaskPayload) ([]openrouter.ChatMessage, error) {
	sysPrompt, userPrompt, err := s.tm.Render("exercise_v1.tmpl", task)
	if err != nil {
		return nil, fmt.Errorf("failed to render exercise template: %w", err)
	}

	return []openrouter.ChatMessage{
		{Role: "system", Content: sysPrompt},
		{Role: "user", Content: userPrompt},
	}, nil
}

func (s *ExerciseStrategy) ValidateAndParse(raw []byte) (interface{}, error) {
	cleaned := CleanJSONResponse(string(raw))

	var resp domain.ExerciseResponse
	if err := json.Unmarshal(cleaned, &resp); err != nil {
		return nil, fmt.Errorf("json unmarshal failed: %w", err)
	}

	// Schema validations
	if strings.TrimSpace(resp.Name) == "" {
		return nil, fmt.Errorf("missing or empty 'name' field")
	}
	if strings.TrimSpace(resp.Category) == "" {
		return nil, fmt.Errorf("missing or empty 'category' field")
	}
	if strings.TrimSpace(resp.MuscleGroup) == "" {
		return nil, fmt.Errorf("missing or empty 'muscle_group' field")
	}
	if len(resp.Instructions) == 0 {
		return nil, fmt.Errorf("'instructions' array must contain at least one step")
	}

	return resp, nil
}

func (s *ExerciseStrategy) SchemaDefinition() string {
	return `{
  "name": "string (required, non-empty)",
  "category": "string (required, e.g. 'strength', 'cardio')",
  "muscle_group": "string (required, e.g. 'Hamstrings, Glutes')",
  "difficulty": "string (required, e.g. 'intermediate')",
  "equipment_needed": "string (e.g. 'Barbell')",
  "instructions": [
    "string (required, at least one step)"
  ],
  "tips": [
    "string (optional tips)"
  ]
}`
}

