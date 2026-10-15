package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/wkt84/esapi-cli/cmd"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	exit := cmd.RunContext(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(exit)
}
