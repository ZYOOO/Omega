package omegalocal

import "fmt"

func makePipeline(item map[string]any) map[string]any {
	template := findPipelineTemplate("feature")
	if template == nil {
		template = &PipelineTemplate{ID: "feature", Name: "Feature delivery", Description: "Default feature delivery flow.", StageProfiles: defaultStageProfiles()}
	}
	return makePipelineWithTemplate(item, template)
}

func makePipelineWithTemplate(item map[string]any, template *PipelineTemplate) map[string]any {
	createdAt := nowISO()
	runID := fmt.Sprintf("run_%s", text(item, "id"))
	stages := stagesFromTemplate(template)
	agents := agentContractsForStages(stages)
	return map[string]any{
		"id":         fmt.Sprintf("pipeline_%s", text(item, "id")),
		"workItemId": text(item, "id"),
		"runId":      runID,
		"status":     "draft",
		"templateId": template.ID,
		"run": map[string]any{
			"id": runID,
			"requirement": map[string]any{
				"id":          stringOr(text(item, "requirementId"), fmt.Sprintf("req_%s", text(item, "id"))),
				"identifier":  text(item, "key"),
				"title":       text(item, "title"),
				"description": text(item, "description"),
				"source":      stringOr(text(item, "source"), "manual"),
				"priority":    "high",
				"requester":   text(item, "assignee"),
				"labels":      item["labels"],
				"createdAt":   createdAt,
			},
			"goal":            fmt.Sprintf("Deliver %s: %s", text(item, "key"), text(item, "title")),
			"successCriteria": []any{"All pipeline stages are passed", "All human gates are approved", "Testing and review evidence is attached", "Delivery notes and rollback plan are attached"},
			"stages":          stages,
			"agents":          agents,
			"orchestrator": map[string]any{
				"masterAgentId":      "master",
				"dispatchStatus":     "ready",
				"templateId":         template.ID,
				"repositoryTargetId": text(item, "repositoryTargetId"),
			},
			"workflow": map[string]any{
				"id":             template.ID,
				"name":           template.Name,
				"source":         template.Source,
				"states":         template.StateProfiles,
				"actions":        workflowActionPlan(template),
				"taskClasses":    template.TaskClasses,
				"hooks":          template.Hooks,
				"reviewRounds":   template.ReviewRounds,
				"runtime":        template.Runtime,
				"transitions":    template.Transitions,
				"promptSections": template.PromptSections,
				"executionMode":  workflowExecutionMode(template),
			},
			"dataFlow":             dataFlowForStages(stages),
			"selectedCapabilities": map[string]any{"llmProvider": defaultProviderSelection().ProviderID, "model": defaultProviderSelection().Model},
			"events": []map[string]any{
				{"id": fmt.Sprintf("event_%s_1", runID), "type": "run.created", "message": fmt.Sprintf("Pipeline created for %s", text(item, "key")), "timestamp": createdAt, "stageId": "intake", "agentId": "master"},
				{"id": fmt.Sprintf("event_%s_2", runID), "type": "master.dispatch.created", "message": fmt.Sprintf("Master agent dispatched %d stage agent contract(s)", len(agents)), "timestamp": createdAt, "stageId": "orchestration", "agentId": "master"},
			},
			"createdAt": createdAt,
			"updatedAt": createdAt,
		},
		"createdAt": createdAt,
		"updatedAt": createdAt,
	}
}

func normalizePipelineExecutionMetadata(database WorkspaceDatabase) WorkspaceDatabase {
	timestamp := nowISO()
	for index, pipeline := range database.Tables.Pipelines {
		template := findPipelineTemplate(text(pipeline, "templateId"))
		if template == nil {
			continue
		}
		item := findWorkItem(database, text(pipeline, "workItemId"))
		if item == nil {
			item = map[string]any{"id": text(pipeline, "workItemId"), "key": text(pipeline, "workItemId")}
		}
		normalized := makePipelineWithTemplate(item, template)
		next := cloneMap(pipeline)
		run := mapValue(next["run"])
		normalizedRun := mapValue(normalized["run"])
		run["stages"] = mergeStageRuntimeState(arrayMaps(run["stages"]), arrayMaps(normalizedRun["stages"]))
		if len(arrayMaps(run["agents"])) == 0 {
			run["agents"] = normalizedRun["agents"]
		}
		if len(arrayMaps(run["dataFlow"])) == 0 {
			run["dataFlow"] = normalizedRun["dataFlow"]
		}
		if len(mapValue(run["orchestrator"])) == 0 {
			run["orchestrator"] = normalizedRun["orchestrator"]
		}
		workflow := mapValue(run["workflow"])
		normalizedWorkflow := mapValue(normalizedRun["workflow"])
		if len(workflow) == 0 {
			run["workflow"] = normalizedWorkflow
		} else {
			if len(arrayMaps(workflow["reviewRounds"])) == 0 {
				workflow["reviewRounds"] = normalizedWorkflow["reviewRounds"]
			}
			if len(arrayMaps(workflow["states"])) == 0 {
				workflow["states"] = normalizedWorkflow["states"]
			}
			if len(arrayMaps(workflow["actions"])) == 0 {
				workflow["actions"] = normalizedWorkflow["actions"]
			}
			if len(arrayMaps(workflow["taskClasses"])) == 0 {
				workflow["taskClasses"] = normalizedWorkflow["taskClasses"]
			}
			if len(mapValue(workflow["hooks"])) == 0 {
				workflow["hooks"] = normalizedWorkflow["hooks"]
			}
			if len(arrayMaps(workflow["transitions"])) == 0 {
				workflow["transitions"] = normalizedWorkflow["transitions"]
			}
			if len(mapValue(workflow["runtime"])) == 0 {
				workflow["runtime"] = normalizedWorkflow["runtime"]
			}
			if len(mapValue(workflow["promptSections"])) == 0 {
				workflow["promptSections"] = normalizedWorkflow["promptSections"]
			}
			if text(workflow, "executionMode") == "" {
				workflow["executionMode"] = normalizedWorkflow["executionMode"]
			}
			run["workflow"] = workflow
		}
		if len(mapValue(run["selectedCapabilities"])) == 0 {
			run["selectedCapabilities"] = normalizedRun["selectedCapabilities"]
		}
		if len(arrayMaps(run["events"])) == 0 {
			run["events"] = normalizedRun["events"]
		}
		if len(mapValue(run["requirement"])) == 0 {
			run["requirement"] = normalizedRun["requirement"]
		}
		next["run"] = run
		next["updatedAt"] = stringOr(text(next, "updatedAt"), timestamp)
		database.Tables.Pipelines[index] = next
	}
	return database
}

func applyWorkflowTemplateToPipeline(pipeline map[string]any, template PipelineTemplate) map[string]any {
	next := cloneMap(pipeline)
	run := mapValue(next["run"])
	workflow := mapValue(run["workflow"])
	workflow["id"] = template.ID
	workflow["name"] = template.Name
	workflow["source"] = template.Source
	workflow["states"] = template.StateProfiles
	workflow["actions"] = workflowActionPlan(&template)
	workflow["taskClasses"] = template.TaskClasses
	workflow["hooks"] = template.Hooks
	workflow["reviewRounds"] = template.ReviewRounds
	workflow["runtime"] = template.Runtime
	workflow["transitions"] = template.Transitions
	workflow["promptSections"] = template.PromptSections
	workflow["executionMode"] = workflowExecutionMode(&template)
	run["workflow"] = workflow
	next["run"] = run
	next["updatedAt"] = nowISO()
	return next
}

func mergeStageRuntimeState(existing []map[string]any, normalized []map[string]any) []map[string]any {
	existingByID := map[string]map[string]any{}
	for _, stage := range existing {
		existingByID[text(stage, "id")] = stage
	}
	result := make([]map[string]any, 0, len(normalized))
	for _, base := range normalized {
		next := cloneMap(base)
		if prior := existingByID[text(base, "id")]; prior != nil {
			for _, key := range []string{"status", "startedAt", "completedAt", "notes", "evidence", "acceptanceCriteria", "approvedBy", "rejectionReason"} {
				if prior[key] != nil {
					next[key] = prior[key]
				}
			}
		}
		result = append(result, next)
	}
	return result
}

func defaultStages() []map[string]any {
	return stagesFromTemplate(&PipelineTemplate{StageProfiles: defaultStageProfiles()})
}

func stage(id, title, agent string, humanGate bool, status string) map[string]any {
	return map[string]any{"id": id, "name": title, "title": title, "description": title, "agentId": agent, "ownerAgentId": agent, "status": status, "humanGate": humanGate, "dependsOn": []any{}, "inputArtifacts": []any{}, "outputArtifacts": stageOutputArtifacts(id), "acceptanceCriteria": []any{"Criteria is satisfied"}, "evidence": []any{}}
}

type StageProfile struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Agent           string   `json:"agentId"`
	AgentIDs        []string `json:"agents,omitempty"`
	HumanGate       bool     `json:"humanGate"`
	InputArtifacts  []string `json:"inputArtifacts,omitempty"`
	OutputArtifacts []string `json:"outputArtifacts,omitempty"`
}

type PipelineTemplate struct {
	ID               string                      `json:"id"`
	Name             string                      `json:"name"`
	Description      string                      `json:"description"`
	Source           string                      `json:"source,omitempty"`
	PromptTemplate   string                      `json:"promptTemplate,omitempty"`
	WorkflowMarkdown string                      `json:"workflowMarkdown,omitempty"`
	StageProfiles    []StageProfile              `json:"stages"`
	StateProfiles    []WorkflowStateProfile      `json:"states,omitempty"`
	TaskClasses      []WorkflowTaskClassProfile  `json:"taskClasses,omitempty"`
	Hooks            WorkflowHookProfile         `json:"hooks,omitempty"`
	ReviewRounds     []ReviewRoundProfile        `json:"reviewRounds,omitempty"`
	Runtime          WorkflowRuntimeProfile      `json:"runtime,omitempty"`
	Transitions      []WorkflowTransitionProfile `json:"transitions,omitempty"`
	PromptSections   map[string]string           `json:"promptSections,omitempty"`
}

func isDevFlowPRTemplate(templateID string) bool {
	return templateID == "devflow-pr"
}

func pipelineTemplates() []PipelineTemplate {
	workflowTemplates := workflowPipelineTemplates()
	devflowTemplate, hasWorkflowDevFlow := firstTemplateByID(workflowTemplates, "devflow-pr")
	templates := []PipelineTemplate{
		{
			ID:            "feature",
			Name:          "Feature delivery",
			Description:   "Full requirement to delivery flow for new product capabilities.",
			StageProfiles: defaultStageProfiles(),
		},
	}
	if hasWorkflowDevFlow {
		templates = append(templates, devflowTemplate)
	} else {
		templates = append(templates, PipelineTemplate{
			ID:          "devflow-pr",
			Name:        "DevFlow PR cycle",
			Description: "Local-first Omega flow: intake, implementation, two code review rounds, human review, merge, done.",
			StageProfiles: []StageProfile{
				{ID: "todo", Title: "Todo intake", Agent: "requirement", HumanGate: false},
				{ID: "in_progress", Title: "Implementation and PR", Agent: "coding", HumanGate: false, AgentIDs: []string{"architect", "coding", "testing"}},
				{ID: "code_review_round_1", Title: "Code Review Round 1", Agent: "review", HumanGate: false},
				{ID: "code_review_round_2", Title: "Code Review Round 2", Agent: "review", HumanGate: false},
				{ID: "rework", Title: "Rework", Agent: "coding", HumanGate: false, AgentIDs: []string{"coding", "testing"}},
				{ID: "human_review", Title: "Human Review", Agent: "delivery", HumanGate: true, AgentIDs: []string{"human", "review", "delivery"}},
				{ID: "merging", Title: "Merging", Agent: "delivery", HumanGate: false},
				{ID: "done", Title: "Done", Agent: "delivery", HumanGate: false},
			},
			ReviewRounds: defaultDevFlowReviewRounds(),
			Runtime:      WorkflowRuntimeProfile{MaxReviewCycles: 3, RunnerHeartbeatSeconds: 10, AttemptTimeoutMinutes: 30, MaxRetryAttempts: 2, RetryBackoffSeconds: 300, CleanupRetentionSeconds: 86400, MaxContinuationTurns: 2},
		})
	}
	templates = append(templates,
		PipelineTemplate{
			ID:          "bugfix",
			Name:        "Bug fix",
			Description: "Tighter flow focused on reproduction, patching, regression tests, and review.",
			StageProfiles: []StageProfile{
				{ID: "intake", Title: "Reproduce", Agent: "requirement", HumanGate: true},
				{ID: "coding", Title: "Patch", Agent: "coding", HumanGate: false},
				{ID: "testing", Title: "Regression", Agent: "testing", HumanGate: true},
				{ID: "review", Title: "Review", Agent: "review", HumanGate: true},
				{ID: "delivery", Title: "Delivery", Agent: "delivery", HumanGate: true},
			},
		},
		PipelineTemplate{
			ID:          "refactor",
			Name:        "Refactor",
			Description: "Architecture-sensitive flow with explicit solution and review gates.",
			StageProfiles: []StageProfile{
				{ID: "intake", Title: "Scope", Agent: "requirement", HumanGate: true},
				{ID: "solution", Title: "Design", Agent: "architect", HumanGate: true},
				{ID: "coding", Title: "Refactor", Agent: "coding", HumanGate: false},
				{ID: "testing", Title: "Safety checks", Agent: "testing", HumanGate: true},
				{ID: "review", Title: "Architecture review", Agent: "review", HumanGate: true},
				{ID: "delivery", Title: "Delivery", Agent: "delivery", HumanGate: true},
			},
		},
	)
	return templates
}

func firstTemplateByID(templates []PipelineTemplate, id string) (PipelineTemplate, bool) {
	for _, template := range templates {
		if template.ID == id {
			return template, true
		}
	}
	return PipelineTemplate{}, false
}

func removeDuplicateTemplateIDs(templates []PipelineTemplate) []PipelineTemplate {
	seen := map[string]bool{}
	output := make([]PipelineTemplate, 0, len(templates))
	for _, template := range templates {
		if template.ID == "" || seen[template.ID] {
			continue
		}
		seen[template.ID] = true
		output = append(output, template)
	}
	return output
}

func defaultDevFlowReviewRounds() []ReviewRoundProfile {
	return []ReviewRoundProfile{
		{StageID: "code_review_round_1", Artifact: "code-review-round-1.md", Focus: "correctness, regressions, and acceptance criteria", DiffSource: "local_diff", ChangesRequestedTo: "rework", NeedsHumanInfoTo: "human_review"},
		{StageID: "code_review_round_2", Artifact: "code-review-round-2.md", Focus: "maintainability, tests, edge cases, and delivery readiness", DiffSource: "pr_diff", ChangesRequestedTo: "rework", NeedsHumanInfoTo: "human_review"},
	}
}

func defaultStageProfiles() []StageProfile {
	return []StageProfile{
		{ID: "intake", Title: "Intake", Agent: "requirement", HumanGate: true},
		{ID: "solution", Title: "Solution", Agent: "architect", HumanGate: true},
		{ID: "coding", Title: "Implementation", Agent: "coding", HumanGate: false},
		{ID: "testing", Title: "Testing", Agent: "testing", HumanGate: true},
		{ID: "review", Title: "Review", Agent: "review", HumanGate: true},
		{ID: "delivery", Title: "Delivery", Agent: "delivery", HumanGate: true},
	}
}

func findPipelineTemplate(templateID string) *PipelineTemplate {
	if templateID == "" {
		templateID = "feature"
	}
	for _, template := range pipelineTemplates() {
		if template.ID == templateID {
			return &template
		}
	}
	return nil
}

func stagesFromTemplate(template *PipelineTemplate) []map[string]any {
	stages := make([]map[string]any, 0, len(template.StageProfiles))
	for index, profile := range template.StageProfiles {
		status := "waiting"
		if index == 0 {
			status = "ready"
		}
		nextStage := stage(profile.ID, profile.Title, profile.Agent, profile.HumanGate, status)
		nextStage["agentIds"] = stageAgentIDs(profile)
		if len(profile.OutputArtifacts) > 0 {
			nextStage["outputArtifacts"] = anyListFromStrings(profile.OutputArtifacts)
		}
		if len(profile.InputArtifacts) > 0 {
			nextStage["inputArtifacts"] = anyListFromStrings(profile.InputArtifacts)
		}
		if index > 0 {
			previous := stages[index-1]
			nextStage["dependsOn"] = []any{text(previous, "id")}
			if len(profile.InputArtifacts) == 0 {
				nextStage["inputArtifacts"] = previous["outputArtifacts"]
			}
		} else {
			if len(profile.InputArtifacts) == 0 {
				nextStage["inputArtifacts"] = []any{"raw-requirement", "repository-target"}
			}
		}
		stages = append(stages, nextStage)
	}
	return stages
}

func stageAgentIDs(profile StageProfile) []any {
	if len(profile.AgentIDs) > 0 {
		return anyListFromStrings(profile.AgentIDs)
	}
	switch profile.ID {
	case "in_progress":
		return []any{"architect", "coding", "testing"}
	case "human_review":
		return []any{"review", "delivery"}
	case "merging", "done":
		return []any{"delivery"}
	default:
		return []any{profile.Agent}
	}
}

func agentContractsForStages(stages []map[string]any) []map[string]any {
	selection := defaultProviderSelection()
	definitions := agentDefinitions(selection)
	contractsByID := map[string]map[string]any{}
	for _, definition := range definitions {
		contractsByID[definition.ID] = map[string]any{
			"id":             definition.ID,
			"name":           definition.Name,
			"stageId":        definition.StageID,
			"systemPrompt":   definition.SystemPrompt,
			"inputContract":  definition.InputContract,
			"outputContract": definition.OutputContract,
			"defaultTools":   definition.DefaultTools,
			"defaultModel":   definition.DefaultModel,
		}
	}
	contracts := []map[string]any{contractsByID["master"]}
	seen := map[string]bool{"master": true}
	for _, stage := range stages {
		agentIDs := anySlice(stage["agentIds"])
		if len(agentIDs) == 0 {
			agentIDs = []any{text(stage, "ownerAgentId")}
		}
		for _, value := range agentIDs {
			agentID := fmt.Sprint(value)
			if seen[agentID] {
				continue
			}
			if contract := contractsByID[agentID]; contract != nil {
				contracts = append(contracts, contract)
				seen[agentID] = true
			}
		}
	}
	return contracts
}

func dataFlowForStages(stages []map[string]any) []map[string]any {
	flows := []map[string]any{}
	for index := 1; index < len(stages); index++ {
		from := stages[index-1]
		to := stages[index]
		flows = append(flows, map[string]any{
			"fromStageId": text(from, "id"),
			"toStageId":   text(to, "id"),
			"artifacts":   from["outputArtifacts"],
		})
	}
	return flows
}

func stageOutputArtifacts(stageID string) []any {
	switch stageID {
	case "intake", "todo":
		return []any{"structured-requirement", "acceptance-criteria", "dispatch-plan"}
	case "solution":
		return []any{"technical-plan", "file-change-list", "test-strategy"}
	case "coding", "in_progress":
		return []any{"code-diff", "changed-files", "implementation-notes"}
	case "testing":
		return []any{"test-report", "coverage-risk-notes"}
	case "review", "code_review_round_1", "code_review_round_2":
		return []any{"review-report", "blocking-risks", "merge-recommendation"}
	case "human_review":
		return []any{"human-decision", "review-notes"}
	case "merging", "delivery":
		return []any{"pull-request", "delivery-summary", "rollback-plan"}
	case "done":
		return []any{"handoff-bundle", "proof-records"}
	default:
		return []any{stageID + "-artifact"}
	}
}

func makeMission(item map[string]any) map[string]any {
	stageID := text(item, "stageId")
	if stageID == "" {
		stageID = "intake"
	}
	agent := text(item, "assignee")
	if agent == "" {
		agent = "requirement"
	}
	status := "ready"
	if text(item, "status") == "Done" {
		status = "done"
	}
	return map[string]any{
		"id":                    fmt.Sprintf("mission_%s_%s", text(item, "key"), stageID),
		"sourceIssueKey":        text(item, "key"),
		"sourceWorkItemId":      text(item, "id"),
		"title":                 text(item, "title"),
		"target":                text(item, "target"),
		"repositoryTargetId":    text(item, "repositoryTargetId"),
		"repositoryTargetLabel": text(item, "repositoryTargetLabel"),
		"status":                status,
		"checkpointRequired":    stageID != "coding",
		"operations": []map[string]any{{
			"id":            fmt.Sprintf("operation_%s", stageID),
			"stageId":       stageID,
			"agentId":       agent,
			"status":        status,
			"prompt":        fmt.Sprintf("Mission: %s\nSource work item: %s\nStage: %s\nAgent: %s\nRepository target ID: %s\nRepository target: %s\nRepository label: %s", text(item, "title"), text(item, "key"), stageID, agent, stringOr(text(item, "repositoryTargetId"), "unscoped"), text(item, "target"), text(item, "repositoryTargetLabel")),
			"requiredProof": []any{"proof"},
		}},
		"links": []any{},
	}
}
