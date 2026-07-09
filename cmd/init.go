package cmd

import (
	"embed"
	"fmt"
	"os"

	"github.com/kirillbaranov/figma-map/internal/scaffold"
	"github.com/kirillbaranov/figma-map/internal/service"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

// newInitCmd bootstraps figma-map into a target project: the skill, a
// starter config, MCP server registration, and a CLAUDE.md section. It's
// hand-registered here (like newMCPCmd) rather than routed through the
// internal/op registry, since it's inherently an interactive terminal
// wizard with no sensible single-shot MCP-tool shape.
func newInitCmd(_ func() *service.Service, assets embed.FS) *cobra.Command {
	var force, yes bool

	cmd := &cobra.Command{
		Use:   "init [path]",
		Short: "Install the figma-map skill, config, and MCP registration into a project",
		Long: "init drops the bundled Claude Code skill and a starter figma-map.yaml " +
			"into a target project, registers figma-map as an MCP server in that " +
			"project's .mcp.json, and adds a short figma-map section to its " +
			"CLAUDE.md. Give it a path directly for non-interactive use, or run it " +
			"with no arguments in a terminal to pick a project interactively.",
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, args []string) error {
			target, err := resolveTarget(args)
			if err != nil {
				return err
			}

			statuses, err := planWrites(assets, target, force)
			if err != nil {
				return err
			}

			fmt.Println("figma-map init:", target)
			for _, s := range statuses {
				fmt.Println("  ·", s)
			}

			if !yes {
				ok, err := scaffold.Confirm("Proceed?")
				if err != nil {
					if err == scaffold.ErrCancelled {
						fmt.Println("cancelled — nothing written")
						return nil
					}
					return err
				}
				if !ok {
					fmt.Println("cancelled — nothing written")
					return nil
				}
			}

			binaryPath, err := os.Executable()
			if err != nil {
				binaryPath = "figma-map"
			}

			results := []string{}
			run := func(status string, err error) error {
				if err != nil {
					return err
				}
				results = append(results, status)
				return nil
			}
			if err := run(scaffold.WriteSkill(assets, target, force, false)); err != nil {
				return err
			}
			if err := run(scaffold.WriteConfig(assets, target, false)); err != nil {
				return err
			}
			if err := run(scaffold.WriteMCPConfig(target, binaryPath, force, false)); err != nil {
				return err
			}
			if err := run(scaffold.WriteClaudeSection(target, false)); err != nil {
				return err
			}

			fmt.Println()
			for _, r := range results {
				fmt.Println("  ✓", r)
			}
			printNextSteps(target)
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "overwrite files that differ from the bundled version")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")

	return cmd
}

// resolveTarget returns the target project directory: the positional arg if
// given, or an interactively picked one. It refuses to guess a default when
// stdin isn't a real terminal, rather than silently defaulting to cwd.
func resolveTarget(args []string) (string, error) {
	if len(args) == 1 {
		return args[0], nil
	}
	if !isatty.IsTerminal(os.Stdin.Fd()) {
		return "", fmt.Errorf("init requires a target path when not running interactively")
	}

	candidates := scaffold.Discover(scaffold.CandidateRoots(), 2)
	target, err := scaffold.PickProjectDir(candidates)
	if err != nil {
		if err == scaffold.ErrCancelled {
			return "", fmt.Errorf("cancelled")
		}
		return "", err
	}
	return target, nil
}

// planWrites previews every write init would perform, without touching the
// filesystem, so the confirmation prompt shows an accurate summary.
func planWrites(assets embed.FS, target string, force bool) ([]string, error) {
	var out []string
	steps := []func() (string, error){
		func() (string, error) { return scaffold.WriteSkill(assets, target, force, true) },
		func() (string, error) { return scaffold.WriteConfig(assets, target, true) },
		func() (string, error) {
			return scaffold.WriteMCPConfig(target, "<resolved at write time>", force, true)
		},
		func() (string, error) { return scaffold.WriteClaudeSection(target, true) },
	}
	for _, step := range steps {
		status, err := step()
		if err != nil {
			return nil, err
		}
		out = append(out, status)
	}
	return out, nil
}

func printNextSteps(target string) {
	fmt.Println()
	fmt.Println("Next:")
	fmt.Printf("  cd %s\n", target)
	fmt.Println("  export OPENAI_API_KEY=sk-...        # edit figma-map.yaml if the defaults don't fit")
	fmt.Println("  figma-map bridge up --repo <path>   # start the backend (set bridgeRepo in figma-map.yaml to skip --repo)")
	fmt.Println("  figma-map doctor                    # verify bridge, chrome, storybook, key")
	fmt.Println("  figma-map setup scan --project .    # build the code-component catalog")
	fmt.Println("  figma-map setup bind                # match Figma components to it")
	fmt.Println()
	fmt.Println("Your AI agent should now see figma-map's MCP tools automatically")
	fmt.Println("(restart it if it already had this project open).")
}
