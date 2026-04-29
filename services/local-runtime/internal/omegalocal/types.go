package omegalocal

import "encoding/json"

type WorkspaceDatabase struct {
	SchemaVersion int             `json:"schemaVersion"`
	SavedAt       string          `json:"savedAt"`
	Tables        WorkspaceTables `json:"tables"`
}

type WorkspaceTables struct {
	Projects             []map[string]any `json:"projects"`
	Requirements         []map[string]any `json:"requirements"`
	WorkItems            []map[string]any `json:"workItems"`
	MissionControlStates []map[string]any `json:"missionControlStates"`
	MissionEvents        []map[string]any `json:"missionEvents"`
	SyncIntents          []map[string]any `json:"syncIntents"`
	Connections          []map[string]any `json:"connections"`
	UIPreferences        []map[string]any `json:"uiPreferences"`
	Pipelines            []map[string]any `json:"pipelines"`
	Attempts             []map[string]any `json:"attempts"`
	Checkpoints          []map[string]any `json:"checkpoints"`
	Missions             []map[string]any `json:"missions"`
	Operations           []map[string]any `json:"operations"`
	ProofRecords         []map[string]any `json:"proofRecords"`
	RunWorkpads          []map[string]any `json:"runWorkpads"`
}

type RuntimeLogRecord struct {
	ID                 string         `json:"id"`
	Level              string         `json:"level"`
	EventType          string         `json:"eventType"`
	Message            string         `json:"message"`
	EntityType         string         `json:"entityType,omitempty"`
	EntityID           string         `json:"entityId,omitempty"`
	ProjectID          string         `json:"projectId,omitempty"`
	RepositoryTargetID string         `json:"repositoryTargetId,omitempty"`
	WorkItemID         string         `json:"workItemId,omitempty"`
	PipelineID         string         `json:"pipelineId,omitempty"`
	AttemptID          string         `json:"attemptId,omitempty"`
	StageID            string         `json:"stageId,omitempty"`
	AgentID            string         `json:"agentId,omitempty"`
	RequestID          string         `json:"requestId,omitempty"`
	Details            map[string]any `json:"details,omitempty"`
	CreatedAt          string         `json:"createdAt"`
}

type AttemptTimelineItem struct {
	ID           string         `json:"id"`
	Time         string         `json:"time"`
	Source       string         `json:"source"`
	Level        string         `json:"level"`
	EventType    string         `json:"eventType"`
	Message      string         `json:"message"`
	StageID      string         `json:"stageId,omitempty"`
	AgentID      string         `json:"agentId,omitempty"`
	OperationID  string         `json:"operationId,omitempty"`
	ProofID      string         `json:"proofId,omitempty"`
	CheckpointID string         `json:"checkpointId,omitempty"`
	RuntimeLogID string         `json:"runtimeLogId,omitempty"`
	Details      map[string]any `json:"details,omitempty"`
}

type AttemptTimelineResponse struct {
	Attempt     map[string]any        `json:"attempt"`
	Pipeline    map[string]any        `json:"pipeline,omitempty"`
	Items       []AttemptTimelineItem `json:"items"`
	GeneratedAt string                `json:"generatedAt"`
}

type OperationResult struct {
	OperationID   string           `json:"operationId"`
	Status        string           `json:"status"`
	WorkspacePath string           `json:"workspacePath"`
	ProofFiles    []string         `json:"proofFiles"`
	Stdout        string           `json:"stdout"`
	Stderr        string           `json:"stderr"`
	BranchName    string           `json:"branchName,omitempty"`
	CommitSha     string           `json:"commitSha,omitempty"`
	ChangedFiles  []string         `json:"changedFiles,omitempty"`
	RunnerProcess map[string]any   `json:"runnerProcess,omitempty"`
	Events        []map[string]any `json:"events"`
}

func cloneMap(input map[string]any) map[string]any {
	raw, _ := json.Marshal(input)
	var output map[string]any
	_ = json.Unmarshal(raw, &output)
	if output == nil {
		return map[string]any{}
	}
	return output
}
