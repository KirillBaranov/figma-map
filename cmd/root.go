// Package cmd wires the figma-map CLI subcommands.
package cmd

import (
	"github.com/kirillbaranov/figma-map/internal/config"
	"github.com/spf13/cobra"
)

// configPath is the --config flag value, shared by all subcommands.
var configPath string

// loadConfig loads the config file named by the --config flag.
func loadConfig() (config.Config, error) {
	return config.Load(configPath)
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "figma-map",
		Short: "Map Figma design components to code components",
		Long: "figma-map matches Figma components to a Storybook component library " +
			"using a vision LLM, writes a reviewable binding, and generates code.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&configPath, "config", "figma-map.yaml", "path to config file")

	root.AddCommand(
		newDoctorCmd(),
		newScanCmd(),
		newBindCmd(),
		newMapCmd(),
	)
	return root
}

// Execute runs the root command.
func Execute() error {
	return newRootCmd().Execute()
}
