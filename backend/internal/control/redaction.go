package control

import (
	"regexp"
	"strings"
)

var evidenceSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]{8,}`),
	regexp.MustCompile(`(?i)\b(?:sk|ghp|github_pat|barena_pat)[_-][A-Za-z0-9_-]{8,}`),
	regexp.MustCompile(`(?i)(authorization|api[_-]?key|access[_-]?token|secret|password)(\s*[=:]\s*)[^\s,;"'}]+`),
}

func redactEvidenceText(value string) string {
	result := strings.ToValidUTF8(value, "�")
	for _, pattern := range evidenceSecretPatterns {
		result = pattern.ReplaceAllStringFunc(result, func(match string) string {
			if index := strings.IndexAny(match, "=:"); index >= 0 {
				return match[:index+1] + "[REDACTED]"
			}
			if strings.HasPrefix(strings.ToLower(match), "bearer ") {
				return "Bearer [REDACTED]"
			}
			return "[REDACTED]"
		})
	}
	return result
}
