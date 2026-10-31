package strategy

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/allfitness/ai-gateway/internal/domain"
	"github.com/allfitness/ai-gateway/internal/openrouter"
	"github.com/allfitness/ai-gateway/internal/prompt"
)

// RecipeStrategy handles GENERATE_RECIPE requests.
type RecipeStrategy struct {
	tm *prompt.TemplateManager
}

// NewRecipeStrategy creates a new RecipeStrategy.
func NewRecipeStrategy(tm *prompt.TemplateManager) *RecipeStrategy {
	return &RecipeStrategy{tm: tm}
}

func (s *RecipeStrategy) RequestType() domain.RequestType {
	return domain.RequestTypeRecipe
}

func (s *RecipeStrategy) BuildMessages(task *domain.TaskPayload) ([]openrouter.ChatMessage, error) {
	sysPrompt, userPrompt, err := s.tm.Render("recipe_v1.tmpl", task)
	if err != nil {
		return nil, fmt.Errorf("failed to render recipe template: %w", err)
	}

	return []openrouter.ChatMessage{
		{Role: "system", Content: sysPrompt},
		{Role: "user", Content: userPrompt},
	}, nil
}

func (s *RecipeStrategy) ValidateAndParse(raw []byte) (interface{}, error) {
	cleaned := CleanJSONResponse(string(raw))

	// Flexible parsing for ingredients: string or array of strings
	type RecipeHelper struct {
		Title           string          `json:"title"`
		Description     string          `json:"description"`
		IngredientsRaw  json.RawMessage `json:"ingredients"`
		PreparationTime int             `json:"preparation_time"`
		Calories        int             `json:"calories"`
		Protein         float64         `json:"protein"`
		Carbs           float64         `json:"carbs"`
		Fats            float64         `json:"fats"`
		Instructions    []string        `json:"instructions"`
	}

	var helper RecipeHelper
	if err := json.Unmarshal(cleaned, &helper); err != nil {
		return nil, fmt.Errorf("json unmarshal failed: %w", err)
	}

	resp := domain.RecipeResponse{
		Title:           helper.Title,
		Description:     helper.Description,
		PreparationTime: helper.PreparationTime,
		Calories:        helper.Calories,
		Protein:         helper.Protein,
		Carbs:           helper.Carbs,
		Fats:            helper.Fats,
		Instructions:    helper.Instructions,
	}

	// Unmarshal ingredients (handle both string and []string)
	if len(helper.IngredientsRaw) > 0 {
		var strIng string
		if err := json.Unmarshal(helper.IngredientsRaw, &strIng); err == nil {
			resp.Ingredients = strIng
		} else {
			var listIng []string
			if errList := json.Unmarshal(helper.IngredientsRaw, &listIng); errList == nil {
				resp.Ingredients = strings.Join(listIng, "\n")
			} else {
				return nil, fmt.Errorf("'ingredients' must be a formatted string")
			}
		}
	}

	// Schema validations
	if strings.TrimSpace(resp.Title) == "" {
		return nil, fmt.Errorf("missing or empty 'title' field")
	}
	if strings.TrimSpace(resp.Description) == "" {
		return nil, fmt.Errorf("missing or empty 'description' field")
	}
	if strings.TrimSpace(resp.Ingredients) == "" {
		return nil, fmt.Errorf("missing or empty 'ingredients' field")
	}
	if resp.PreparationTime <= 0 {
		return nil, fmt.Errorf("'preparation_time' must be a positive integer, got %d", resp.PreparationTime)
	}
	if resp.Calories <= 0 {
		return nil, fmt.Errorf("'calories' must be a positive integer, got %d", resp.Calories)
	}
	if resp.Protein < 0 || resp.Carbs < 0 || resp.Fats < 0 {
		return nil, fmt.Errorf("macronutrients (protein, carbs, fats) cannot be negative")
	}
	if len(resp.Instructions) == 0 {
		return nil, fmt.Errorf("'instructions' array must contain at least one step")
	}

	return resp, nil
}

func (s *RecipeStrategy) SchemaDefinition() string {
	return `{
  "title": "string (required, non-empty)",
  "description": "string (required, non-empty)",
  "ingredients": "string (required, multiline formatted ingredients)",
  "preparation_time": "integer > 0 (required)",
  "calories": "integer > 0 (required)",
  "protein": "number >= 0 (required)",
  "carbs": "number >= 0 (required)",
  "fats": "number >= 0 (required)",
  "instructions": [
    "string (required, at least one step)"
  ]
}`
}
