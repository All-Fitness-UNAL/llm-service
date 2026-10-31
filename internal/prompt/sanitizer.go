package prompt

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/allfitness/ai-gateway/internal/domain"
)

// Sanitizer provides prompt injection detection and PII scrubbing.
type Sanitizer struct {
	injectionPatterns []*regexp.Regexp
	emailRegex        *regexp.Regexp
	phoneRegex        *regexp.Regexp
	ssnRegex          *regexp.Regexp
	creditCardRegex   *regexp.Regexp
}

// NewSanitizer initializes regular expressions for injection defense and PII redaction.
func NewSanitizer() *Sanitizer {
	// Known prompt injection phrases, instruction overrides, jailbreak vectors, and delimiters
	patterns := []string{
		// System override & instruction bypass
		`(?i)\bignore\s+(all\s+)?(previous|prior|above)\s+(instructions|directives|rules|prompts)\b`,
		`(?i)\bdisregard\s+(all\s+)?(previous|prior|above)\s+(instructions|directives|rules|prompts)\b`,
		`(?i)\bforget\s+(all\s+)?(previous|prior|above)\s+(instructions|directives|rules|prompts)\b`,
		`(?i)\boverride\s+(system|safety|security)\s+(instructions|filters|rules|prompts)\b`,
		`(?i)\bbypass\s+(content\s+)?(filter|guardrail|policy|rules)\b`,

		// Jailbreaks and persona switching
		`(?i)\byou\s+are\s+now\s+(DAN|unrestricted|evil|an\s+AI\s+without\s+rules|ChatGPT)\b`,
		`(?i)\bact\s+as\s+(an?\s+)?(unrestricted|jailbroken|DAN|evil|developer\s+mode)\b`,
		`(?i)\bdeveloper\s+mode\s+(enabled|on|activate)\b`,
		`(?i)\bDAN\s+mode\b`,
		`(?i)\bdo\s+anything\s+now\b`,

		// Prompt extraction attacks
		`(?i)\b(reveal|print|display|output|show|repeat|tell|give)\s+(?:[a-z0-9_-]+\s+){0,3}(?:the\s+)?(system\s+prompt|initial\s+prompt|hidden\s+instructions|system\s+message|system\s+instructions)\b`,
		`(?i)\bwhat\s+(?:are\s+your|is\s+your)\s+(?:original|system|hidden)\s+(?:instructions|prompt)\b`,

		// LLM delimiter attacks and token forgery
		`(?i)<\|im_start\|>`,
		`(?i)<\|im_end\|>`,
		`(?i)<\|system\|>`,
		`(?i)<\|user\|>`,
		`(?i)<\|assistant\|>`,
		`(?i)\[INST\]`,
		`(?i)\[/INST\]`,
		`(?i)<<SYS>>`,
		`(?i)<</SYS>>`,

		// Inappropriate and exploit injection
		`(?i)\b(sudo|chmod\s+777|rm\s+-rf|DROP\s+TABLE|SELECT\s+\*\s+FROM)\b`,
	}

	compiledPatterns := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		compiledPatterns = append(compiledPatterns, regexp.MustCompile(p))
	}

	return &Sanitizer{
		injectionPatterns: compiledPatterns,
		emailRegex:        regexp.MustCompile(`(?i)\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`),
		phoneRegex:        regexp.MustCompile(`(?:\+?(\d{1,3}))?[-. (]*(\d{3})[-. )]*(\d{3})[-. ]*(\d{4})(?: *x(\d+))?\b`),
		ssnRegex:          regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),
		creditCardRegex:   regexp.MustCompile(`\b(?:\d{4}[ -]?){3}\d{4}\b`),
	}
}

// CheckInjection scans the text against prompt injection patterns.
// Returns an error if any malicious pattern is detected.
func (s *Sanitizer) CheckInjection(text string) error {
	if text == "" {
		return nil
	}

	// Normalize text to counter simple evasion techniques (e.g. multiple spaces, zero-width chars)
	normalized := s.normalizeText(text)

	for _, pattern := range s.injectionPatterns {
		if pattern.MatchString(normalized) {
			return fmt.Errorf("%w: input matches injection rule %s", domain.ErrPromptInjectionDetected, pattern.String())
		}
	}

	return nil
}

// SanitizePII removes sensitive personal identifiable information (emails, phones, SSNs, credit cards).
func (s *Sanitizer) SanitizePII(text string) string {
	if text == "" {
		return ""
	}

	res := s.emailRegex.ReplaceAllString(text, "[EMAIL_REDACTED]")
	res = s.creditCardRegex.ReplaceAllString(res, "[CARD_REDACTED]")
	res = s.ssnRegex.ReplaceAllString(res, "[SSN_REDACTED]")
	res = s.phoneRegex.ReplaceAllString(res, "[PHONE_REDACTED]")

	return res
}

// SanitizeUserProfile validates prompt injection defense and scrubs PII from user inputs.
func (s *Sanitizer) SanitizeUserProfile(profile *domain.UserProfile) error {
	fieldsToCheck := []struct {
		name string
		val  *string
	}{
		{"diet_description", &profile.DietDescription},
		{"objective_description", &profile.ObjectiveDescription},
		{"goal", &profile.Goal},
	}

	// 1. Run Injection defense
	for _, field := range fieldsToCheck {
		if err := s.CheckInjection(*field.val); err != nil {
			return fmt.Errorf("%w in field '%s'", domain.ErrPromptInjectionDetected, field.name)
		}
	}

	// 2. Scrub PII
	for _, field := range fieldsToCheck {
		*field.val = s.SanitizePII(*field.val)
	}

	return nil
}

// normalizeText cleans zero-width spaces and collapses consecutive whitespace.
func (s *Sanitizer) normalizeText(text string) string {
	// Strip non-printable / zero-width characters
	var b strings.Builder
	b.Grow(len(text))

	for _, r := range text {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			continue
		}
		// Zero width spaces
		if r == '\u200B' || r == '\u200C' || r == '\u200D' || r == '\uFEFF' {
			continue
		}
		b.WriteRune(r)
	}

	// Collapse multiple spaces
	cleaned := b.String()
	spaceRegex := regexp.MustCompile(`[ \t]+`)
	return spaceRegex.ReplaceAllString(cleaned, " ")
}
