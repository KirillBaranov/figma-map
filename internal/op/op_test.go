package op

import (
	"context"
	"testing"

	"github.com/kirillbaranov/figma-map/internal/service"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

// TestConvergence is the no-drift guarantee: every operation surfaces as both a
// CLI subcommand and an MCP tool with identical name and description.
func TestConvergence(t *testing.T) {
	ops := All()
	if len(ops) == 0 {
		t.Fatal("no operations registered")
	}

	// --- CLI side: build commands, find each op by its (possibly nested) path ---
	root := &cobra.Command{Use: "figma-map"}
	get := func() *service.Service { return nil } // not invoked in this test
	for _, o := range ops {
		o.AddCLI(root, get)
	}

	// --- MCP side: register tools, list them over an in-memory transport ---
	srv := mcp.NewServer(&mcp.Implementation{Name: "figma-map", Version: "test"}, nil)
	for _, o := range ops {
		o.AddMCP(srv, nil) // handlers not invoked; registration only
	}

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer func() { _ = ss.Close() }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() { _ = cs.Close() }()

	listed, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	mcpDesc := map[string]string{}
	for _, tool := range listed.Tools {
		mcpDesc[tool.Name] = tool.Description
	}

	// --- assert every op converges across both surfaces ---
	for _, o := range ops {
		name, summary := o.Meta()
		path := o.CLIPath()

		cmd, _, err := root.Find(path)
		if err != nil || cmd.Name() != path[len(path)-1] {
			t.Errorf("op %q has no CLI command at path %v", name, path)
		} else if cmd.Short != summary {
			t.Errorf("op %q CLI Short %q != summary %q", name, cmd.Short, summary)
		}

		desc, ok := mcpDesc[name]
		if !ok {
			t.Errorf("op %q has no MCP tool", name)
		} else if desc != summary {
			t.Errorf("op %q MCP description %q != summary %q", name, desc, summary)
		}
	}

	if len(mcpDesc) != len(ops) {
		t.Errorf("MCP tool count %d != op count %d", len(mcpDesc), len(ops))
	}
}
