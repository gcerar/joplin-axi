package commands

import (
	"context"

	"github.com/gcerar/joplin-axi/internal/args"
	"github.com/gcerar/joplin-axi/internal/client"
	"github.com/gcerar/joplin-axi/internal/toon"
)

var pingSpec = args.CommandSpec{
	Name:     "ping",
	Summary:  "Check connectivity and authentication against the Joplin Web Clipper API.",
	Usage:    "joplin-axi ping",
	Flags:    nil,
	Examples: []string{"joplin-axi ping"},
}

func runPing(ctx context.Context, _ args.ParsedArgs, c client.Client) (CommandResult, error) {
	clipperOK := c.Ping(ctx)
	auth := "failed"

	if clipperOK {
		if _, err := c.ListNotebooks(ctx, nil); err == nil {
			auth = "ok"
		}
	}

	clipperStatus := "unreachable"
	if clipperOK {
		clipperStatus = "reachable"
	}

	return Ok(toon.Object("ping", []toon.Field{
		{Key: "clipper", Value: clipperStatus},
		{Key: "auth", Value: auth},
	})), nil
}

// PingCommand checks connectivity and authentication against the Joplin
// Web Clipper API.
var PingCommand = Command{Spec: pingSpec, Run: runPing}
