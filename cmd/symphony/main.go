package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"gameproject/internal/symphony"
)

const guardrailsFlag = "i-understand-that-this-will-be-running-without-the-usual-guardrails"

func main() {
	var logsRoot string
	var port int
	var acknowledge bool

	flag.StringVar(&logsRoot, "logs-root", "./log", "directory for Symphony logs")
	flag.IntVar(&port, "port", -1, "HTTP status/API port; overrides workflow server.port when >= 0")
	flag.BoolVar(&acknowledge, guardrailsFlag, false, "acknowledge this preview runs Codex without the usual product guardrails")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: symphony [--logs-root <path>] [--port <port>] [--%s] [path-to-WORKFLOW.md]\n", guardrailsFlag)
		flag.PrintDefaults()
	}
	flag.Parse()

	if !acknowledge {
		fmt.Fprintln(os.Stderr, acknowledgementBanner())
		os.Exit(1)
	}

	workflowPath := "WORKFLOW.md"
	if flag.NArg() > 1 {
		flag.Usage()
		os.Exit(1)
	}
	if flag.NArg() == 1 {
		workflowPath = flag.Arg(0)
	}

	workflowPath, err := filepath.Abs(workflowPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid workflow path: %v\n", err)
		os.Exit(1)
	}
	if info, err := os.Stat(workflowPath); err != nil || info.IsDir() {
		fmt.Fprintf(os.Stderr, "Workflow file not found: %s\n", workflowPath)
		os.Exit(1)
	}

	logsRoot, err = filepath.Abs(logsRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid logs root: %v\n", err)
		os.Exit(1)
	}

	logger, err := symphony.NewLogger(logsRoot, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logging: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	service := symphony.NewService(symphony.ServiceOptions{
		WorkflowPath: workflowPath,
		LogsRoot:     logsRoot,
		PortOverride: port,
		Logger:       logger,
	})

	if err := service.Run(ctx); err != nil {
		logger.Error("service stopped with error", "error", err)
		os.Exit(1)
	}
}

func acknowledgementBanner() string {
	return "This Symphony Go implementation is an engineering preview.\n" +
		"Codex will run in unattended app-server sessions using the policy in WORKFLOW.md.\n" +
		"To proceed, pass --" + guardrailsFlag + "."
}
