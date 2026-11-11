package control

import (
	"path"
	"regexp"
	"strings"
)

type harnessSkill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"`
}

var skillName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

func validManagedProfile(skills []harnessSkill, knowledge []string) bool {
	if len(skills) > 8 || len(knowledge) > 16 {
		return false
	}
	names := map[string]bool{}
	total := 0
	for _, skill := range skills {
		total += len(skill.Content) + len(skill.Description)
		if !skillName.MatchString(skill.Name) || names[skill.Name] ||
			strings.TrimSpace(skill.Description) == "" || len(skill.Description) > 200 ||
			strings.TrimSpace(skill.Content) == "" || total > 32*1024 {
			return false
		}
		names[skill.Name] = true
	}
	for _, ref := range knowledge {
		if len(ref) > 256 || ref == "" || ref == "." || ref == ".." ||
			path.IsAbs(ref) || path.Clean(ref) != ref || strings.HasPrefix(ref, "../") ||
			strings.ContainsAny(ref, "\\:\x00\r\n") {
			return false
		}
	}
	return true
}
