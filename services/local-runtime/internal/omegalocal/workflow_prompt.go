package omegalocal

import "strings"

func renderWorkflowPromptSection(template *PipelineTemplate, section string, variables map[string]string, fallback string) string {
	if template == nil || template.PromptSections == nil {
		return fallback
	}
	text := strings.TrimSpace(template.PromptSections[section])
	if text == "" {
		return fallback
	}
	for key, value := range variables {
		text = strings.ReplaceAll(text, "{{"+key+"}}", value)
	}
	return text
}

func cloneStringMap(input map[string]string) map[string]string {
	output := map[string]string{}
	for key, value := range input {
		output[key] = value
	}
	return output
}
