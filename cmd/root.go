// Package cmd wires the figma-map CLI. Every subcommand is generated from the
// shared operation registry (internal/op), so the CLI and the MCP server expose
// the same operations with identical names and descriptions.
package cmd

import (
	"embed"
	"fmt"

	"github.com/kirillbaranov/figma-map/internal/config"
	"github.com/kirillbaranov/figma-map/internal/op"
	"github.com/kirillbaranov/figma-map/internal/service"
	"github.com/spf13/cobra"
)

var configPath string

// BuildInfo carries version metadata injected at build time.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

func newRootCmd(info BuildInfo, assets embed.FS) *cobra.Command {
	var svc *service.Service

	root := &cobra.Command{
		Use:   "figma-map",
		Short: "Map Figma design components to code components",
		Long: "figma-map matches Figma components to a Storybook component library " +
			"using a vision LLM, writes a reviewable binding, and generates code.",
		Version:       fmt.Sprintf("%s (commit %s, built %s)", info.Version, info.Commit, info.Date),
		SilenceUsage:  true,
		SilenceErrors: true,
		// Load config after flags are parsed, then build the service once.
		// .env is loaded first so OPENAI_API_KEY (and any other secret) is
		// picked up automatically without the caller having to export it.
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			if err := config.LoadEnvFile(".env"); err != nil {
				return fmt.Errorf("load .env: %w", err)
			}
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			svc = service.New(cfg)
			return nil
		},
	}
	root.PersistentFlags().StringVar(&configPath, "config", "figma-map.yaml", "path to config file")

	get := func() *service.Service { return svc }
	for _, o := range op.All() {
		o.AddCLI(root, get)
	}
	root.AddCommand(newMCPCmd(get))
	root.AddCommand(newInitCmd(get, assets))
	root.AddCommand(newUpdateCmd(info))

	return root
}

// Execute runs the root command.
func Execute(info BuildInfo, assets embed.FS) error {
	return newRootCmd(info, assets).Execute()
}
