package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// RequestType represents the supported AI generation task types.
type RequestType string

const (
	RequestTypeWorkout  RequestType = "GENERATE_WORKOUT"
	RequestTypeRecipe   RequestType = "GENERATE_RECIPE"
	RequestTypeExercise RequestType = "GENERATE_EXERCISE"
)

// Standard Error Codes
const (
	ErrCodePromptInjection      = "PROMPT_INJECTION_DETECTED"
	ErrCodeInvalidPayload       = "INVALID_PAYLOAD"
	ErrCodeUnsupportedType      = "UNSUPPORTED_REQUEST_TYPE"
	ErrCodeLLMError             = "LLM_API_ERROR"
	ErrCodeSchemaValidationFail = "SCHEMA_VALIDATION_FAILED"
	ErrCodeInternalError        = "INTERNAL_ERROR"
)

// Domain sentinel errors
var (
	ErrPromptInjectionDetected = errors.New("prompt injection detected in user input")
	ErrUnsupportedRequestType  = errors.New("unsupported request type")
	ErrEmptyPayload            = errors.New("task payload is empty")
)

// UserProfile captures user-specific fitness, biometric, and dietary attributes.
type UserProfile struct {
	UserID               string  `json:"user_id"`
	Age                  int     `json:"age"`
	Height               float64 `json:"height"`
	Weight               float64 `json:"weight"`
	ExperienceLevel      string  `json:"experience_level"`
	Goal                 string  `json:"goal"`
	TimeAvailability     int     `json:"time_availability"` // in minutes
	DietDescription      string  `json:"diet_description"`
	ObjectiveDescription string  `json:"objective_description"`
}

// TaskPayload represents the expected incoming JSON read from the Redis task queue.
type TaskPayload struct {
	RequestID          string                 `json:"request_id"`
	RequestType        RequestType            `json:"request_type"`
	CreatedAt          time.Time              `json:"created_at"`
	UserProfile        UserProfile            `json:"user_profile"`
	AvailableEquipment []string               `json:"available_equipment"`
	Parameters         map[string]interface{} `json:"parameters,omitempty"`
}

// Custom unmarshaler for TaskPayload to handle multiple date formats gracefully.
func (t *TaskPayload) UnmarshalJSON(data []byte) error {
	type Alias TaskPayload
	aux := &struct {
		CreatedAtRaw interface{} `json:"created_at"`
		*Alias
	}{
		Alias: (*Alias)(t),
	}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	switch v := aux.CreatedAtRaw.(type) {
	case string:
		if parsed, err := time.Parse(time.RFC3339, v); err == nil {
			t.CreatedAt = parsed
		} else if parsedNano, err := time.Parse(time.RFC3339Nano, v); err == nil {
			t.CreatedAt = parsedNano
		} else {
			t.CreatedAt = time.Now().UTC()
		}
	default:
		t.CreatedAt = time.Now().UTC()
	}

	return nil
}

// Validate checks essential fields in TaskPayload.
func (t *TaskPayload) Validate() error {
	if t.RequestID == "" {
		return fmt.Errorf("missing request_id")
	}
	switch t.RequestType {
	case RequestTypeWorkout, RequestTypeRecipe, RequestTypeExercise:
		// valid
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedRequestType, t.RequestType)
	}
	return nil
}

// --- Strict LLM Output Schemas ---

// WorkoutExercise represents an individual exercise in a generated workout.
type WorkoutExercise struct {
	ExerciseName string `json:"exercise_name"`
	Sets         int    `json:"sets"`
	Reps         string `json:"reps"` // String allows "8-12", "failure", or "10"
	RestSeconds  int    `json:"rest_seconds"`
	Notes        string `json:"notes,omitempty"`
}

// WorkoutResponse defines the strict schema for GENERATE_WORKOUT.
type WorkoutResponse struct {
	Name              string            `json:"name"`
	EstimatedDuration int               `json:"estimated_duration"` // minutes
	Exercises         []WorkoutExercise `json:"exercises"`
}

// RecipeResponse defines the strict schema for GENERATE_RECIPE.
type RecipeResponse struct {
	Title           string   `json:"title"`
	Description     string   `json:"description"`
	Ingredients     string   `json:"ingredients"` // Formatted string listing ingredients with quantities
	PreparationTime int      `json:"preparation_time"` // minutes
	Calories        int      `json:"calories"`
	Protein         float64  `json:"protein"` // grams
	Carbs           float64  `json:"carbs"`   // grams
	Fats            float64  `json:"fats"`    // grams
	Instructions    []string `json:"instructions"`
}

// ExerciseResponse defines the strict schema for GENERATE_EXERCISE.
type ExerciseResponse struct {
	Name            string   `json:"name"`
	Category        string   `json:"category"`
	MuscleGroup     string   `json:"muscle_group"`
	Difficulty      string   `json:"difficulty"`
	EquipmentNeeded string   `json:"equipment_needed"`
	Instructions    []string `json:"instructions"`
	Tips            []string `json:"tips"`
}

// --- Redis Result Envelope ---

// TaskStatus defines the processing status stored in Redis.
type TaskStatus string

const (
	StatusSuccess TaskStatus = "SUCCESS"
	StatusFailed  TaskStatus = "FAILED"
)

// TaskResult is stored under `allfitness:results:{request_id}` in Redis.
type TaskResult struct {
	RequestID       string          `json:"request_id"`
	RequestType     RequestType     `json:"request_type"`
	Status          TaskStatus      `json:"status"`
	Data            json.RawMessage `json:"data,omitempty"`
	ErrorCode       string          `json:"error_code,omitempty"`
	ErrorMessage    string          `json:"error_message,omitempty"`
	ExecutionTimeMs int64           `json:"execution_time_ms"`
	ModelUsed       string          `json:"model_used,omitempty"`
	RepairAttempts  int             `json:"repair_attempts"`
	CompletedAt     time.Time       `json:"completed_at"`
}
