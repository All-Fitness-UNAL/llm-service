package strategy

import (
	"testing"

	"github.com/allfitness/ai-gateway/internal/domain"
	"github.com/allfitness/ai-gateway/internal/prompt"
)

func TestCleanJSONResponse(t *testing.T) {
	rawWithFences := "```json\n{\"name\": \"Morning Run\"}\n```"
	cleaned := CleanJSONResponse(rawWithFences)
	if string(cleaned) != "{\"name\": \"Morning Run\"}" {
		t.Errorf("unexpected cleaned result: %s", string(cleaned))
	}
}

func TestWorkoutStrategy_ValidateAndParse(t *testing.T) {
	tm, err := prompt.NewTemplateManager("../../prompts")
	if err != nil {
		t.Fatalf("failed to init tm: %v", err)
	}

	strat := NewWorkoutStrategy(tm)

	// Valid payload
	validJSON := []byte(`{
		"name": "Full Body Hypertrophy",
		"estimated_duration": 45,
		"exercises": [
			{"exercise_name": "Squat", "sets": 3, "reps": "8-10", "rest_seconds": 90}
		]
	}`)

	res, err := strat.ValidateAndParse(validJSON)
	if err != nil {
		t.Fatalf("expected valid parse, got error: %v", err)
	}
	workout, ok := res.(domain.WorkoutResponse)
	if !ok || workout.Name != "Full Body Hypertrophy" {
		t.Errorf("unexpected parsed workout: %+v", res)
	}

	// Invalid payload: empty exercises
	invalidJSON := []byte(`{
		"name": "Empty Workout",
		"estimated_duration": 30,
		"exercises": []
	}`)
	_, err = strat.ValidateAndParse(invalidJSON)
	if err == nil {
		t.Errorf("expected error for empty exercises, got nil")
	}

	// Invalid payload: negative duration
	negDurationJSON := []byte(`{
		"name": "Negative Duration",
		"estimated_duration": -5,
		"exercises": [{"exercise_name": "Lunge", "sets": 2, "reps": "12", "rest_seconds": 60}]
	}`)
	_, err = strat.ValidateAndParse(negDurationJSON)
	if err == nil {
		t.Errorf("expected error for negative duration, got nil")
	}
}

func TestRecipeStrategy_ValidateAndParse(t *testing.T) {
	tm, err := prompt.NewTemplateManager("../../prompts")
	if err != nil {
		t.Fatalf("failed to init tm: %v", err)
	}

	strat := NewRecipeStrategy(tm)

	validJSON := []byte(`{
		"title": "Berry Protein Smoothie",
		"description": "Quick post-workout drink",
		"ingredients": "• 1 scoop Whey Protein\n• 1 cup Almond Milk\n• 1/2 cup Mixed Berries",
		"preparation_time": 5,
		"calories": 250,
		"protein": 28.0,
		"carbs": 20.0,
		"fats": 4.5,
		"instructions": ["Blend all ingredients until smooth."]
	}`)

	res, err := strat.ValidateAndParse(validJSON)
	if err != nil {
		t.Fatalf("expected valid recipe parse, got error: %v", err)
	}
	recipe, ok := res.(domain.RecipeResponse)
	if !ok || recipe.Title != "Berry Protein Smoothie" {
		t.Errorf("unexpected parsed recipe: %+v", res)
	}

	// Also test flexible ingredients array unmarshaling
	validArrayIngredients := []byte(`{
		"title": "Berry Smoothie",
		"description": "Tasty",
		"ingredients": ["1 scoop Whey", "1 cup Milk"],
		"preparation_time": 5,
		"calories": 200,
		"protein": 25.0,
		"carbs": 15.0,
		"fats": 3.0,
		"instructions": ["Blend."]
	}`)
	resArr, errArr := strat.ValidateAndParse(validArrayIngredients)
	if errArr != nil {
		t.Fatalf("expected valid parse with array ingredients, got error: %v", errArr)
	}
	recipeArr := resArr.(domain.RecipeResponse)
	if recipeArr.Ingredients != "1 scoop Whey\n1 cup Milk" {
		t.Errorf("unexpected ingredients format: %s", recipeArr.Ingredients)
	}
}
