// Command tdm is the Tandem CLI. All logic is in internal/cli.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/0mega24/tandem/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := cli.Run(ctx, os.Args, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
