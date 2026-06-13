package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/kirillbaranov/figma-map/internal/op"
	"github.com/kirillbaranov/figma-map/internal/service"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

// newMCPCmd starts an MCP server over stdio exposing every operation as a tool.
// The same op registry backs the CLI, so tools and commands never diverge.
func newMCPCmd(get func() *service.Service) *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Run figma-map as an MCP server over stdio (for AI agents)",
		Long: "mcp exposes figma-map's operations as Model Context Protocol tools " +
			"over stdio. Configure your agent with: \n" +
			`  { "mcpServers": { "figma-map": { "command": "figma-map", "args": ["mcp"] } } }`,
		RunE: func(c *cobra.Command, _ []string) error {
			srv := mcp.NewServer(&mcp.Implementation{
				Name:    "figma-map",
				Version: c.Root().Version,
			}, nil)

			svc := get()
			ops := op.All()
			for _, o := range ops {
				o.AddMCP(srv, svc)
			}
			// Logs go to stderr — stdout carries the MCP protocol.
			fmt.Fprintf(os.Stderr, "figma-map mcp: serving %d tools over stdio\n", len(ops))
			return srv.Run(context.Background(), &mcp.StdioTransport{})
		},
	}
}
