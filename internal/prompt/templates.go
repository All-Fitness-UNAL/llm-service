package prompt

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/allfitness/ai-gateway/internal/domain"
)

//go:embed templates_embedded/*.tmpl
var embeddedTemplates embed.FS

// TemplateManager compiles and renders modular prompt templates.
type TemplateManager struct {
	promptsDir string
	templates  map[string]*template.Template
}

// NewTemplateManager initializes the template manager from the specified directory or embedded assets.
func NewTemplateManager(promptsDir string) (*TemplateManager, error) {
	tm := &TemplateManager{
		promptsDir: promptsDir,
		templates:  make(map[string]*template.Template),
	}

	templateFiles := []string{
		"workout_v1.tmpl",
		"recipe_v1.tmpl",
		"exercise_v1.tmpl",
	}

	for _, name := range templateFiles {
		tmpl, err := tm.loadTemplate(name)
		if err != nil {
			return nil, fmt.Errorf("failed to load template %s: %w", name, err)
		}
		tm.templates[name] = tmpl
	}

	return tm, nil
}

// loadTemplate tries loading from disk first; falls back to embedded templates.
func (tm *TemplateManager) loadTemplate(name string) (*template.Template, error) {
	diskPath := filepath.Join(tm.promptsDir, name)
	if content, err := os.ReadFile(diskPath); err == nil {
		return template.New(name).Parse(string(content))
	}

	// Fallback to embedded
	embeddedPath := filepath.Join("templates_embedded", name)
	if content, err := embeddedTemplates.ReadFile(embeddedPath); err == nil {
		return template.New(name).Parse(string(content))
	}

	return nil, fmt.Errorf("template file not found in disk path %s or embedded", diskPath)
}

// Render compiles the template with the provided payload and splits into system and user prompts.
func (tm *TemplateManager) Render(templateName string, task *domain.TaskPayload) (systemPrompt, userPrompt string, err error) {
	tmpl, ok := tm.templates[templateName]
	if !ok {
		return "", "", fmt.Errorf("template %s not registered", templateName)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, task); err != nil {
		return "", "", fmt.Errorf("failed to execute template %s: %w", templateName, err)
	}

	rendered := buf.String()

	// Parse sections by delimiter ---SYSTEM--- and ---USER---
	systemIndex := strings.Index(rendered, "---SYSTEM---")
	userIndex := strings.Index(rendered, "---USER---")

	if systemIndex != -1 && userIndex != -1 && userIndex > systemIndex {
		systemPrompt = strings.TrimSpace(rendered[systemIndex+len("---SYSTEM---") : userIndex])
		userPrompt = strings.TrimSpace(rendered[userIndex+len("---USER---"):])
	} else {
		// Fallback if delimiters missing
		systemPrompt = "You are a professional All Fitness AI assistant. Respond ONLY with valid JSON."
		userPrompt = strings.TrimSpace(rendered)
	}

	return systemPrompt, userPrompt, nil
}
