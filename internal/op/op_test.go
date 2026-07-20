package op

import (
	"context"
	"strings"
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
		} else if !strings.HasPrefix(desc, summary) {
			// MCP description is summary, plus the op's Long help text
			// appended (see AddMCP) so agents get the same detail CLI
			// --help does — it must still start with the shared summary.
			t.Errorf("op %q MCP description %q doesn't start with summary %q", name, desc, summary)
		}
	}

	if len(mcpDesc) != len(ops) {
		t.Errorf("MCP tool count %d != op count %d", len(mcpDesc), len(ops))
	}
}

// TestAddMCPRendersNarrative asserts the fix for the "dry" agent output: an
// op with a Render func must surface that human-readable text to MCP callers
// (not just CLI ones), without losing the structured JSON output.
func TestAddMCPRendersNarrative(t *testing.T) {
	type out struct{ N int }
	rendered := Op[struct{}, out]{
		Verb:    "rendered",
		Summary: "has a Render func",
		Run: func(context.Context, *service.Service, struct{}) (out, error) {
			return out{N: 3}, nil
		},
		Render: func(o out) string { return "N is 3, should be fixed" },
	}
	bare := Op[struct{}, out]{
		Verb:    "bare",
		Summary: "has no Render func",
		Run: func(context.Context, *service.Service, struct{}) (out, error) {
			return out{N: 5}, nil
		},
	}

	srv := mcp.NewServer(&mcp.Implementation{Name: "figma-map", Version: "test"}, nil)
	rendered.AddMCP(srv, nil)
	bare.AddMCP(srv, nil)

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

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "rendered"})
	if err != nil {
		t.Fatalf("call rendered: %v", err)
	}
	if len(res.Content) != 1 {
		t.Fatalf("rendered: want 1 content block, got %d", len(res.Content))
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok || tc.Text != "N is 3, should be fixed" {
		t.Errorf("rendered: Content = %#v, want the Render() narrative", res.Content[0])
	}
	if res.StructuredContent == nil {
		t.Error("rendered: StructuredContent is nil, want the full JSON output alongside the narrative")
	}

	res2, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "bare"})
	if err != nil {
		t.Fatalf("call bare: %v", err)
	}
	if len(res2.Content) != 1 {
		t.Fatalf("bare: want 1 fallback content block, got %d", len(res2.Content))
	}
	if _, ok := res2.Content[0].(*mcp.TextContent); !ok {
		t.Errorf("bare: Content[0] = %#v, want the SDK's default JSON TextContent fallback", res2.Content[0])
	}
}
