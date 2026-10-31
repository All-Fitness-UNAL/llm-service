package openrouter

import (
	"context"
	"strings"
)

// MockClient simulates OpenRouter responses for local development and testing.
type MockClient struct {
	ShouldFailPrimary  bool
	SimulateInvalidJSON bool
	InvalidJSONCount    int
}

// NewMockClient creates a new mock LLM client.
func NewMockClient() *MockClient {
	return &MockClient{}
}

// GenerateWithFallback returns mock responses conforming to our domain schemas.
func (m *MockClient) GenerateWithFallback(ctx context.Context, messages []ChatMessage) (string, string, error) {
	if m.SimulateInvalidJSON && m.InvalidJSONCount > 0 {
		m.InvalidJSONCount--
		// Return broken JSON to trigger repair
		return `{"name": "Broken Workout", "exercises": [INVALID_JSON_HERE`, "mock/broken-model", nil
	}

	modelUsed := "anthropic/claude-3.5-sonnet"
	if m.ShouldFailPrimary {
		modelUsed = "openai/gpt-4o-mini"
	}

	// Look at the prompt content to determine type
	var allContent strings.Builder
	for _, msg := range messages {
		allContent.WriteString(msg.Content)
		allContent.WriteString(" ")
	}
	contentStr := allContent.String()

	if strings.Contains(contentStr, "Nutritionist") || strings.Contains(contentStr, "recipe") || strings.Contains(contentStr, "ingredients") {
		return `{
  "title": "High-Protein Mediterranean Grilled Chicken & Quinoa Bowl",
  "description": "Nutrient-packed meal providing lean protein, complex carbohydrates, and healthy fats.",
  "ingredients": "• 220g Chicken Breast (skinless)\n• 1 cup Cooked Quinoa\n• 1/2 Avocado (diced)\n• 1 cup Steamed Broccoli\n• 1 tbsp Extra Virgin Olive Oil\n• 1 pinch Sea Salt & Black Pepper",
  "preparation_time": 25,
  "calories": 520,
  "protein": 48.5,
  "carbs": 42.0,
  "fats": 16.5,
  "instructions": [
    "Season chicken breast with olive oil, salt, and black pepper.",
    "Grill chicken over medium-high heat for 6-7 minutes per side until internal temperature reaches 75°C (165°F).",
    "Cook quinoa according to package instructions.",
    "Assemble the bowl with quinoa base, sliced chicken, steamed broccoli, and diced avocado."
  ]
}`, modelUsed, nil
	}

	if strings.Contains(contentStr, "Biomechanics") || strings.Contains(contentStr, "exercise guide") {
		return `{
  "name": "Barbell Romanian Deadlift",
  "category": "strength",
  "muscle_group": "Hamstrings, Glutes, Erector Spinae",
  "difficulty": "intermediate",
  "equipment_needed": "Barbell and Weight Plates",
  "instructions": [
    "Stand upright with feet hip-width apart holding the barbell with an overhand grip.",
    "Hinge at your hips while keeping a soft bend in the knees and maintaining a neutral spine.",
    "Lower the barbell along your shins until you feel a deep stretch in your hamstrings.",
    "Drive through your heels and extend hips forward to return to the starting position."
  ],
  "tips": [
    "Do not round your lower back at the bottom of the movement.",
    "Keep the barbell close to your legs throughout the entire range of motion."
  ]
}`, modelUsed, nil
	}

	// Default: Workout
	return `{
  "name": "Hypertrophy Upper Body Power & Pump",
  "estimated_duration": 45,
  "exercises": [
    {
      "exercise_name": "Incline Dumbbell Bench Press",
      "sets": 4,
      "reps": "8-12",
      "rest_seconds": 90,
      "notes": "Keep shoulder blades retracted and control the eccentric tempo."
    },
    {
      "exercise_name": "Chest-Supported Dumbbell Row",
      "sets": 4,
      "reps": "10-12",
      "rest_seconds": 75,
      "notes": "Squeeze lats at the top of contraction."
    },
    {
      "exercise_name": "Standing Overhead Dumbbell Press",
      "sets": 3,
      "reps": "8-10",
      "rest_seconds": 60,
      "notes": "Engage core and avoid hyper-extending the lumbar spine."
    },
    {
      "exercise_name": "Incline Dumbbell Bicep Curl",
      "sets": 3,
      "reps": "12-15",
      "rest_seconds": 45,
      "notes": "Full stretch at bottom, strict form without swinging."
    }
  ]
}`, modelUsed, nil
}

// RepairJSON returns valid JSON for repaired items in mock mode.
func (m *MockClient) RepairJSON(ctx context.Context, model string, rawOutput string, schemaError error, schemaInstructions string) (string, string, error) {
	// Successfully repair
	return `{
  "name": "Repaired Upper Body Routine",
  "estimated_duration": 40,
  "exercises": [
    {
      "exercise_name": "Push-ups",
      "sets": 3,
      "reps": "15",
      "rest_seconds": 60,
      "notes": "Full depth."
    }
  ]
}`, "mock/repair-model", nil
}
