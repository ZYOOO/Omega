package omegalocal

import (
	"context"
	"time"
)

type JobSupervisorConfig struct {
	Enabled               bool
	Interval              time.Duration
	StaleAfter            time.Duration
	AutoRunReady          bool
	AutoRetryFailed       bool
	AutoCleanupWorkspaces bool
	MaxRetryAttempts      int
	RetryBackoff          time.Duration
	WorkspaceRetention    time.Duration
	ReadyScanItemLimit    int
}

func (server *Server) StartJobSupervisor(parent context.Context, config JobSupervisorConfig) context.CancelFunc {
	if !config.Enabled {
		return func() {}
	}
	interval := config.Interval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if config.ReadyScanItemLimit <= 0 {
		config.ReadyScanItemLimit = 10
	}
	ctx, cancel := context.WithCancel(parent)
	go server.runJobSupervisorLoop(ctx, interval, config)
	return cancel
}

func (server *Server) runJobSupervisorLoop(ctx context.Context, interval time.Duration, config JobSupervisorConfig) {
	server.runJobSupervisorTick(ctx, config, "startup")
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			server.runJobSupervisorTick(ctx, config, "interval")
		}
	}
}

func (server *Server) runJobSupervisorTick(ctx context.Context, config JobSupervisorConfig, trigger string) {
	options := jobSupervisorTickOptions{
		AutoRunReady:          config.AutoRunReady,
		AutoRetryFailed:       config.AutoRetryFailed,
		AutoCleanupWorkspaces: config.AutoCleanupWorkspaces,
		MaxRetryAttempts:      config.MaxRetryAttempts,
		Limit:                 config.ReadyScanItemLimit,
	}
	if config.StaleAfter > 0 {
		options.StaleAfterSeconds = int(config.StaleAfter.Seconds())
	}
	if config.RetryBackoff > 0 {
		options.RetryBackoffSeconds = int(config.RetryBackoff.Seconds())
	}
	if config.WorkspaceRetention > 0 {
		options.WorkspaceRetentionSeconds = int(config.WorkspaceRetention.Seconds())
	}
	summary, err := server.reconcileAttemptIntegrity(ctx, options)
	if err != nil {
		server.logError(ctx, "job_supervisor.tick.failed", err.Error(), map[string]any{"trigger": trigger})
		return
	}
	server.logDebug(ctx, "job_supervisor.tick.completed", "JobSupervisor tick completed.", map[string]any{
		"trigger":            trigger,
		"changed":            intValue(summary["changed"]),
		"stalledAttempts":    intValue(summary["stalledAttempts"]),
		"pendingCheckpoints": intValue(summary["pendingCheckpoints"]),
		"runnableItems":      intValue(summary["runnableItems"]),
		"acceptedReadyRuns":  intValue(summary["acceptedReadyRuns"]),
		"retryableAttempts":  intValue(summary["retryableAttempts"]),
		"acceptedRetries":    intValue(summary["acceptedRetryAttempts"]),
		"cleanedWorkspaces":  intValue(summary["cleanedWorkspaces"]),
		"workflowContracts":  intValue(summary["workflowContractPipelines"]),
		"workflowMissing":    intValue(summary["workflowContractMissing"]),
	})
}
