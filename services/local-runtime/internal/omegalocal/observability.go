package omegalocal

func emptyObservability() map[string]any {
	return map[string]any{
		"counts": map[string]any{
			"workItems":    0,
			"pipelines":    0,
			"checkpoints":  0,
			"missions":     0,
			"operations":   0,
			"proofRecords": 0,
			"events":       0,
		},
		"pipelineStatus":   map[string]int{},
		"checkpointStatus": map[string]int{},
		"operationStatus":  map[string]int{},
		"attention": map[string]any{
			"waitingHuman": 0,
			"failed":       0,
			"blocked":      0,
		},
	}
}

func observabilitySummary(database WorkspaceDatabase) map[string]any {
	pipelineStatus := countBy(database.Tables.Pipelines, "status")
	checkpointStatus := countBy(database.Tables.Checkpoints, "status")
	operationStatus := countBy(database.Tables.Operations, "status")
	workItemStatus := countBy(database.Tables.WorkItems, "status")

	return map[string]any{
		"counts": map[string]any{
			"workItems":    len(database.Tables.WorkItems),
			"pipelines":    len(database.Tables.Pipelines),
			"checkpoints":  len(database.Tables.Checkpoints),
			"missions":     len(database.Tables.Missions),
			"operations":   len(database.Tables.Operations),
			"proofRecords": len(database.Tables.ProofRecords),
			"events":       len(database.Tables.MissionEvents),
		},
		"pipelineStatus":   pipelineStatus,
		"checkpointStatus": checkpointStatus,
		"operationStatus":  operationStatus,
		"workItemStatus":   workItemStatus,
		"attention": map[string]any{
			"waitingHuman": pipelineStatus["waiting-human"] + checkpointStatus["pending"],
			"failed":       pipelineStatus["failed"] + operationStatus["failed"],
			"blocked":      workItemStatus["Blocked"],
		},
	}
}

func countBy(records []map[string]any, key string) map[string]int {
	counts := map[string]int{}
	for _, record := range records {
		counts[text(record, key)]++
	}
	return counts
}
