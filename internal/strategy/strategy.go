package strategy

import (
	"strings"

	"github.com/All-Fitness-UNAL/llm-service/internal/domain"
	"github.com/All-Fitness-UNAL/llm-service/internal/openrouter"
)

// Strategy defines the interface for task-specific prompt construction and output validation.
type Strategy interface {
	// RequestType returns the specific task type handled by this strategy.
	RequestType() domain.RequestType

	// BuildMessages constructs the chat messages (system & user prompts) using templates.
	BuildMessages(task *domain.TaskPayload) ([]openrouter.ChatMessage, error)

	// ValidateAndParse parses raw LLM output into a typed struct and checks strict schema constraints.
	ValidateAndParse(raw []byte) (interface{}, error)

	// SchemaDefinition returns a human-readable schema string for repair prompts.
	SchemaDefinition() string
}

// CleanJSONResponse strips possible markdown fences (` ```json ` or ` ``` `) from LLM output.
func CleanJSONResponse(raw string) []byte {
	trimmed := strings.TrimSpace(raw)
	// Remove leading ```json or ```
	if strings.HasPrefix(trimmed, "```json") {
		trimmed = strings.TrimPrefix(trimmed, "```json")
	} else if strings.HasPrefix(trimmed, "```") {
		trimmed = strings.TrimPrefix(trimmed, "```")
	}

	// Remove trailing ```
	if strings.HasSuffix(trimmed, "```") {
		trimmed = strings.TrimSuffix(trimmed, "```")
	}

	return []byte(strings.TrimSpace(trimmed))
}

