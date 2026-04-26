package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

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
	flag.Parse()

	server := omegalocal.NewServer(*databasePath, *workspaceRoot, *openAPIPath)
	address := fmt.Sprintf("%s:%s", *host, *port)
	fmt.Printf("Omega Go Local Service listening: http://%s\n", address)
	fmt.Printf("Omega SQLite database: %s\n", *databasePath)
	if err := http.ListenAndServe(address, server.Handler()); err != nil {
		log.Fatal(err)
	}
}
