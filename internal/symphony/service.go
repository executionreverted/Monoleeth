package symphony

import (
	"context"
	"fmt"
	"time"
)

type ServiceOptions struct {
	WorkflowPath string
	LogsRoot     string
	PortOverride int
	Logger       *Logger
}

type Service struct {
	options ServiceOptions
}

func NewService(options ServiceOptions) *Service {
	return &Service{options: options}
}

func (s *Service) Run(ctx context.Context) error {
	if err := EnsureWorkflowPath(s.options.WorkflowPath); err != nil {
		return err
	}
	workflow, err := LoadWorkflow(s.options.WorkflowPath)
	if err != nil {
		return fmt.Errorf("load workflow: %w", err)
	}
	logger := s.options.Logger
	if logger == nil {
		return fmt.Errorf("logger is required")
	}
	logger.Info("starting Symphony Go", "workflow", workflow.Path)

	orchestrator := NewOrchestrator(workflow, logger)
	port := workflow.Config.Server.Port
	if s.options.PortOverride >= 0 {
		port = s.options.PortOverride
	}
	var httpServer *HTTPServer
	if port > 0 {
		httpServer = NewHTTPServer(workflow.Config.Server.Host, port, orchestrator, logger)
		if err := httpServer.Start(); err != nil {
			return fmt.Errorf("start HTTP server: %w", err)
		}
	}

	err = orchestrator.Run(ctx)
	if httpServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}
	return err
}
