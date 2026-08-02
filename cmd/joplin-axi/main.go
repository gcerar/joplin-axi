// Command joplin-axi is an AXI-style CLI for Joplin's Web Clipper (Data) API.
package main

import (
	"context"
	"os"

	"github.com/gcerar/joplin-axi/internal/cli"
)

func main() {
	os.Exit(cli.Run(context.Background(), os.Args[1:], os.Stdout, os.Getenv))
}
