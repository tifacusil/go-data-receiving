// Command data-receiving is the entrypoint for the data-receiving microservice:
// it parses binary DTU sensor frames received over TCP, validates them against
// the JSON Schema contracts, and forwards the resulting envelopes to a sink
// (stdout/file/null/rabbitmq). See ROADMAP.md for the implementation milestones.
//
// This entrypoint is intentionally thin: it wires signal handling and delegates
// all real work to internal/app. The composition root lives in app.Run, which
// is the Go equivalent of the conditional DI setup in the C# Program.cs.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/AngelloFD/go-data-receiving/internal/app"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version") {
		fmt.Printf("data-receiving %s\n", version)
		return
	}

	// SIGINT/SIGTERM cancel ctx, which app.Run propagates to every worker for a
	// graceful shutdown (the Go equivalent of IHostedService.StopAsync).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx, version); err != nil {
		fmt.Fprintf(os.Stderr, "data-receiving: fatal: %v\n", err)
		os.Exit(1)
	}
}
