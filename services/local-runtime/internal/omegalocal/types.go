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
