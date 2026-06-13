// Package op is the convergence layer: each operation is declared once as an
// Op[In,Out], and both the CLI subcommand and the MCP tool are generated from
// it. Names, descriptions, and parameters therefore cannot drift between the
// two surfaces. Per ADR-0001, ops carry no logic — they delegate to
// internal/service.
package op

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/kirillbaranov/figma-map/internal/clibind"
	"github.com/kirillbaranov/figma-map/internal/service"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

// Op declares one operation. In is the typed input (its struct tags define both
// cobra flags and the MCP JSON schema); Out is the typed result.
type Op[In, Out any] struct {
	Name    string // CLI subcommand + MCP tool name
	Summary string // cobra Short + MCP Description (one source)
	Long    string // optional extended CLI help

	Run    func(context.Context, *service.Service, In) (Out, error)
	Render func(Out) string                     // human text for the CLI
	Image  func(Out) (data []byte, mime string) // optional: MCP ImageContent
	Status func(Out) error                      // optional: non-nil → failure exit / IsError
}

// Registrar is the non-generic face so heterogeneous ops live in one slice.
// AddCLI takes a provider because the service depends on --config, which cobra
// parses only after command construction; AddMCP gets the already-built service.
type Registrar interface {
	Meta() (name, summary string)
	AddCLI(parent *cobra.Command, get func() *service.Service)
	AddMCP(srv *mcp.Server, svc *service.Service)
}

// Meta returns the op's name and one-line summary (the single source used by
// both the CLI and the MCP tool).
func (o Op[In, Out]) Meta() (string, string) { return o.Name, o.Summary }

// AddCLI builds the cobra subcommand for this op.
func (o Op[In, Out]) AddCLI(parent *cobra.Command, get func() *service.Service) {
	var in In
	var asJSON bool

	cmd := &cobra.Command{
		Use:          o.Name,
		Short:        o.Summary,
		Long:         o.Long,
		SilenceUsage: true,
	}
	apply, err := clibind.Register(cmd, &in)
	if err != nil {
		// Programmer error in an op's In struct — fail loudly at startup.
		panic(fmt.Sprintf("op %q: %v", o.Name, err))
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output result as JSON")

	cmd.RunE = func(c *cobra.Command, args []string) error {
		if err := apply(args); err != nil {
			return err
		}
		// CLI progress goes to stderr so --json stdout stays clean.
		ctx := service.WithProgress(c.Context(), func(m string) {
			fmt.Fprintln(os.Stderr, m)
		})
		out, err := o.Run(ctx, get(), in)
		if err != nil {
			return err
		}
		if asJSON {
			if err := printJSON(out); err != nil {
				return err
			}
		} else if o.Render != nil {
			fmt.Println(o.Render(out))
		}
		if o.Status != nil {
			return o.Status(out)
		}
		return nil
	}
	parent.AddCommand(cmd)
}

// AddMCP registers this op as an MCP tool over the same Run.
func (o Op[In, Out]) AddMCP(srv *mcp.Server, svc *service.Service) {
	mcp.AddTool(srv, &mcp.Tool{Name: o.Name, Description: o.Summary},
		func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
			out, err := o.Run(ctx, svc, in)
			if err != nil {
				return nil, out, err
			}
			res := &mcp.CallToolResult{}
			if o.Image != nil {
				if data, mime := o.Image(out); len(data) > 0 {
					res.Content = append(res.Content, &mcp.ImageContent{Data: data, MIMEType: mime})
				}
			}
			if o.Status != nil {
				if serr := o.Status(out); serr != nil {
					res.IsError = true
					res.Content = append(res.Content, &mcp.TextContent{Text: serr.Error()})
				}
			}
			return res, out, nil
		})
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
