package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	_, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
}
