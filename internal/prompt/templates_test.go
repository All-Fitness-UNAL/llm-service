package prompt

import (
	"strings"
	"testing"
	"time"

	"github.com/allfitness/ai-gateway/internal/domain"
)

func TestTemplateManager_Render(t *testing.T) {
	tm, err := NewTemplateManager("../../prompts")
	if err != nil {
		t.Fatalf("failed to init template manager: %v", err)
	}

	task := &domain.TaskPayload{
		RequestID:   "req-1",
		RequestType: domain.RequestTypeWorkout,
		CreatedAt:   time.Now(),
		UserProfile: domain.UserProfile{
			Age:                  30,
			Height:               180,
			Weight:               78,
			ExperienceLevel:      "intermediate",
			Goal:                 "Muscle Gain",
			TimeAvailability:     45,
			ObjectiveDescription: "Chest and Triceps focus",
		},
		AvailableEquipment: []string{"Dumbbells", "Incline Bench"},
	}

	sys, user, err := tm.Render("workout_v1.tmpl", task)
	if err != nil {
		t.Fatalf("failed to render workout template: %v", err)
	}

	if !strings.Contains(sys, "All Fitness AI Strength & Conditioning Coach") {
		t.Errorf("expected coach persona in system prompt, got: %s", sys)
	}
	if !strings.Contains(user, "Chest and Triceps focus") {
		t.Errorf("expected user objective in user prompt, got: %s", user)
	}
	if !strings.Contains(user, "Dumbbells") {
		t.Errorf("expected equipment in user prompt, got: %s", user)
	}
}
