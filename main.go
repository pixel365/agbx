package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/pixel365/agbx/cmd"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root := cmd.NewRootCommand()
	if err := root.ExecuteContext(ctx); err != nil {
		stop()
		log.Fatalf("%s execute error: %v", root.Use, err)
	}
}
