package main

import (
	"context"
	"os"

	"omega/services/local-runtime/internal/omegacli"
)

func main() {
	os.Exit(omegacli.Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}
