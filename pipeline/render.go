package pipeline

import "strings"

// RenderTemplate replaces {{key}} placeholders with values from vars.
func RenderTemplate(content string, vars map[string]string) string {
	if content == "" || len(vars) == 0 {
		return content
	}

	rendered := content
	for key, value := range vars {
		placeholder := "{{" + key + "}}"
		rendered = strings.ReplaceAll(rendered, placeholder, value)
	}
	return rendered
}
