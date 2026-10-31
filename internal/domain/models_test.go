package domain

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTaskPayload_UnmarshalJSON(t *testing.T) {
	raw := `{
		"request_id": "req-999",
		"request_type": "GENERATE_RECIPE",
		"created_at": "2026-10-30T12:00:00Z",
		"user_profile": {
			"user_id": "u-1",
			"age": 27,
			"height": 175.5,
			"weight": 72.0,
			"experience_level": "intermediate",
			"goal": "Fat Loss",
			"time_availability": 30,
			"diet_description": "Pescatarian",
			"objective_description": "High fiber"
		},
		"available_equipment": ["Oven", "Blender"]
	}`

	var task TaskPayload
	if err := json.Unmarshal([]byte(raw), &task); err != nil {
		t.Fatalf("failed to unmarshal TaskPayload: %v", err)
	}

	if task.RequestID != "req-999" {
		t.Errorf("expected req-999, got %s", task.RequestID)
	}
	if task.RequestType != RequestTypeRecipe {
		t.Errorf("expected GENERATE_RECIPE, got %s", task.RequestType)
	}
	if task.CreatedAt.IsZero() {
		t.Errorf("expected valid CreatedAt timestamp")
	}
	if task.UserProfile.Age != 27 {
		t.Errorf("expected age 27, got %d", task.UserProfile.Age)
	}

	if err := task.Validate(); err != nil {
		t.Errorf("expected valid task, got: %v", err)
	}
}

func TestTaskPayload_ValidateErrors(t *testing.T) {
	// Missing request_id
	t1 := TaskPayload{
		RequestType: RequestTypeWorkout,
	}
	if err := t1.Validate(); err == nil {
		t.Errorf("expected error for missing request_id")
	}

	// Unsupported type
	t2 := TaskPayload{
		RequestID:   "req-1",
		RequestType: "INVALID_TYPE",
	}
	if err := t2.Validate(); err == nil {
		t.Errorf("expected error for unsupported request_type")
	}
}

func TestTaskResult_Serialization(t *testing.T) {
	res := TaskResult{
		RequestID:       "req-123",
		RequestType:     RequestTypeWorkout,
		Status:          StatusSuccess,
		Data:            json.RawMessage(`{"name": "Upper Body"}`),
		ExecutionTimeMs: 125,
		ModelUsed:       "anthropic/claude-3.5-sonnet",
		RepairAttempts:  0,
		CompletedAt:     time.Now().UTC(),
	}

	bytes, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("failed to marshal TaskResult: %v", err)
	}

	var parsed TaskResult
	if err := json.Unmarshal(bytes, &parsed); err != nil {
		t.Fatalf("failed to unmarshal TaskResult: %v", err)
	}

	if parsed.RequestID != "req-123" || parsed.Status != StatusSuccess {
		t.Errorf("unexpected parsed result: %+v", parsed)
	}
}
