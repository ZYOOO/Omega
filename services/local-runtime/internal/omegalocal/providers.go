package omegalocal

type LLMProvider struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Kind         string   `json:"kind"`
	Models       []string `json:"models"`
	DefaultModel string   `json:"defaultModel"`
	EnvHint      string   `json:"envHint"`
}

type ProviderSelection struct {
	ProviderID      string `json:"providerId"`
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoningEffort"`
}

type AgentDefinition struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	StageID        string            `json:"stageId"`
	SystemPrompt   string            `json:"systemPrompt"`
	InputContract  []string          `json:"inputContract"`
	OutputContract []string          `json:"outputContract"`
	DefaultTools   []string          `json:"defaultTools"`
	DefaultModel   ProviderSelection `json:"defaultModel"`
}

const providerSelectionSettingKey = "llm_provider_selection"

func llmProviders() []LLMProvider {
	return []LLMProvider{
		{
			ID:           "openai",
			Name:         "OpenAI",
			Kind:         "openai",
			Models:       []string{"gpt-5.4-mini", "gpt-5.4", "gpt-5.3-codex"},
			DefaultModel: "gpt-5.4-mini",
			EnvHint:      "OPENAI_API_KEY",
		},
		{
			ID:           "openai-compatible",
			Name:         "OpenAI-compatible",
			Kind:         "openai-compatible",
			Models:       []string{"qwen-plus", "deepseek-chat", "moonshot-v1"},
			DefaultModel: "qwen-plus",
			EnvHint:      "OPENAI_COMPATIBLE_BASE_URL + OPENAI_COMPATIBLE_API_KEY",
		},
	}
}

func defaultProviderSelection() ProviderSelection {
	return ProviderSelection{ProviderID: "openai", Model: "gpt-5.4-mini", ReasoningEffort: "medium"}
}

func validateProviderSelection(selection ProviderSelection) bool {
	for _, provider := range llmProviders() {
		if provider.ID != selection.ProviderID {
			continue
		}
		for _, model := range provider.Models {
			if model == selection.Model {
				return selection.ReasoningEffort != ""
			}
		}
	}
	return false
}

func providerSelectionFromMap(record map[string]any) ProviderSelection {
	return ProviderSelection{
		ProviderID:      stringOr(record["providerId"], defaultProviderSelection().ProviderID),
		Model:           stringOr(record["model"], defaultProviderSelection().Model),
		ReasoningEffort: stringOr(record["reasoningEffort"], defaultProviderSelection().ReasoningEffort),
	}
}

func providerSelectionToMap(selection ProviderSelection) map[string]any {
	return map[string]any{"providerId": selection.ProviderID, "model": selection.Model, "reasoningEffort": selection.ReasoningEffort}
}

func agentDefinitions(selection ProviderSelection) []AgentDefinition {
	return []AgentDefinition{
		{
			ID:           "master",
			Name:         "Master Orchestrator Agent",
			StageID:      "orchestration",
			SystemPrompt: "You understand the incoming requirement, choose the pipeline shape, split work into stage contracts, and dispatch specialized agents with repository boundaries.",
			InputContract: []string{
				"Raw requirement or external issue",
				"Repository target and workspace policy",
				"Available pipeline templates",
			},
			OutputContract: []string{
				"Structured requirement",
				"Dispatch plan",
				"Stage agent assignments",
			},
			DefaultTools: []string{"repo-read", "issue-read", "template-select"},
			DefaultModel: selection,
		},
		{
			ID:           "requirement",
			Name:         "Requirement Agent",
			StageID:      "intake",
			SystemPrompt: "You turn raw product intent into structured requirements, acceptance criteria, risks, and open questions.",
			InputContract: []string{
				"Raw requirement text",
				"Existing Workboard item metadata",
				"Optional repository target",
			},
			OutputContract: []string{
				"Structured requirement summary",
				"Acceptance criteria",
				"Repository boundary",
				"Risks, assumptions, and dispatch notes",
			},
			DefaultTools: []string{"repo-read", "issue-read"},
			DefaultModel: selection,
		},
		{
			ID:           "architect",
			Name:         "Solution Agent",
			StageID:      "solution",
			SystemPrompt: "You inspect repository context and produce a technical plan with affected files, APIs, risks, and test strategy.",
			InputContract: []string{
				"Structured requirement",
				"Repository file paths or snippets",
				"Prior stage outputs",
			},
			OutputContract: []string{
				"Approach",
				"Affected areas",
				"Integration risks",
				"Validation plan",
				"Agent handoff",
			},
			DefaultTools: []string{"repo-read", "code-search"},
			DefaultModel: selection,
		},
		{
			ID:           "coding",
			Name:         "Coding Agent",
			StageID:      "coding",
			SystemPrompt: "You implement the approved technical plan inside an isolated workspace and keep the diff reviewable.",
			InputContract: []string{
				"Approved technical plan",
				"Repository workspace path",
				"Test command contract",
			},
			OutputContract: []string{
				"Code diff summary",
				"Changed file list",
				"Implementation notes",
				"Validation run",
				"Known follow-up or risk",
			},
			DefaultTools: []string{"git", "repo-write", "codex"},
			DefaultModel: selection,
		},
		{
			ID:           "testing",
			Name:         "Testing Agent",
			StageID:      "testing",
			SystemPrompt: "You generate and run tests that map directly to acceptance criteria and report failures as actionable proof.",
			InputContract: []string{
				"Code diff summary",
				"Acceptance criteria",
				"Available test commands",
			},
			OutputContract: []string{
				"Status",
				"Commands and results",
				"Acceptance coverage",
				"Failures",
				"Residual risk",
			},
			DefaultTools: []string{"test-runner", "coverage"},
			DefaultModel: selection,
		},
		{
			ID:           "review",
			Name:         "Review Agent",
			StageID:      "review",
			SystemPrompt: "You review diffs for correctness, safety, maintainability, and merge readiness.",
			InputContract: []string{
				"Code diff",
				"Technical plan",
				"Test results",
			},
			OutputContract: []string{
				"Summary",
				"Blocking findings",
				"Validation gaps",
				"Rework instructions",
				"Residual risks",
				"Explicit verdict",
			},
			DefaultTools: []string{"git-diff", "ci-read"},
			DefaultModel: selection,
		},
		{
			ID:           "delivery",
			Name:         "Delivery Agent",
			StageID:      "delivery",
			SystemPrompt: "You assemble the final delivery summary, proof links, rollback notes, and PR/MR narrative.",
			InputContract: []string{
				"Approved review report",
				"Proof records",
				"Repository delivery target",
			},
			OutputContract: []string{
				"Delivery state",
				"What changed",
				"Proof",
				"Rollback plan",
				"Operator notes",
			},
			DefaultTools: []string{"gh", "release-notes"},
			DefaultModel: selection,
		},
	}
}
