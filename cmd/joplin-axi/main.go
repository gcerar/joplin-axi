// Command joplin-axi is an AXI-style CLI for Joplin's Web Clipper (Data) API.
package main

import (
	"context"
	"os"

	"github.com/gcerar/joplin-axi/internal/cli"
)

// version is set via -ldflags "-X main.version=..." at build time (see
// .goreleaser.yaml) and copied into cli.Version before dispatch.
var version = "dev"

func main() {
	cli.Version = version
	os.Exit(cli.Run(context.Background(), os.Args[1:], os.Stdout, os.Getenv))
}
