package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"

	"gameproject/internal/game/server"
)

func main() {
	config := server.ConfigFromEnv()
	gameServer, err := server.New(config)
	if err != nil {
		log.Fatalf("configure game server: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("game server listening on %s", config.Addr)
	if err := gameServer.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("game server stopped: %v", err)
	}
}
