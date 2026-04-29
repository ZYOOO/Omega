package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"omega/services/local-runtime/internal/omegalocal"
)

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = os.TempDir()
	}
	defaultWorkspace := filepath.Join(home, "Omega", "workspaces")
	defaultDatabase := filepath.Join(cwd, ".omega", "omega.db")
	defaultOpenAPI := filepath.Join(cwd, "docs", "openapi.yaml")

	host := flag.String("host", "127.0.0.1", "host to bind")
	port := flag.String("port", "3888", "port to bind")
	workspaceRoot := flag.String("workspace-root", defaultWorkspace, "local runner workspace root")
	databasePath := flag.String("database", defaultDatabase, "SQLite database path")
	openAPIPath := flag.String("openapi", defaultOpenAPI, "OpenAPI YAML path")
	jobSupervisor := flag.Bool("job-supervisor", true, "run the background JobSupervisor maintenance loop")
	jobSupervisorInterval := flag.Duration("job-supervisor-interval", 30*time.Second, "background JobSupervisor tick interval")
	jobSupervisorStaleAfter := flag.Duration("job-supervisor-stale-after", 15*time.Minute, "mark running attempts stalled after this heartbeat age")
	jobSupervisorAutoRunReady := flag.Bool("job-supervisor-auto-run-ready", false, "allow JobSupervisor to start Ready work items")
	jobSupervisorAutoRetryFailed := flag.Bool("job-supervisor-auto-retry-failed", false, "allow JobSupervisor to retry failed or stalled attempts")
	jobSupervisorAutoCleanupWorkspaces := flag.Bool("job-supervisor-auto-cleanup-workspaces", false, "allow JobSupervisor to remove eligible completed repository checkouts while retaining proof")
	jobSupervisorMaxRetryAttempts := flag.Int("job-supervisor-max-retry-attempts", 0, "maximum automatic retries per attempt root; 0 uses workflow contract")
	jobSupervisorRetryBackoff := flag.Duration("job-supervisor-retry-backoff", 0, "minimum age before JobSupervisor retries a failed or stalled attempt; 0 uses workflow contract")
	jobSupervisorWorkspaceRetention := flag.Duration("job-supervisor-workspace-retention", 0, "minimum age before completed workspaces are cleanup candidates; 0 uses workflow contract")
	jobSupervisorLimit := flag.Int("job-supervisor-limit", 10, "maximum Ready work items to scan per JobSupervisor tick")
	flag.Parse()

	server := omegalocal.NewServer(*databasePath, *workspaceRoot, *openAPIPath)
	stopSupervisor := server.StartJobSupervisor(context.Background(), omegalocal.JobSupervisorConfig{
		Enabled:               *jobSupervisor,
		Interval:              *jobSupervisorInterval,
		StaleAfter:            *jobSupervisorStaleAfter,
		AutoRunReady:          *jobSupervisorAutoRunReady,
		AutoRetryFailed:       *jobSupervisorAutoRetryFailed,
		AutoCleanupWorkspaces: *jobSupervisorAutoCleanupWorkspaces,
		MaxRetryAttempts:      *jobSupervisorMaxRetryAttempts,
		RetryBackoff:          *jobSupervisorRetryBackoff,
		WorkspaceRetention:    *jobSupervisorWorkspaceRetention,
		ReadyScanItemLimit:    *jobSupervisorLimit,
	})
	defer stopSupervisor()
	address := fmt.Sprintf("%s:%s", *host, *port)
	fmt.Printf("Omega Go Local Service listening: http://%s\n", address)
	fmt.Printf("Omega SQLite database: %s\n", *databasePath)
	if err := http.ListenAndServe(address, server.Handler()); err != nil {
		log.Fatal(err)
	}
}
