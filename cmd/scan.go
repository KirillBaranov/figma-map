package cmd

import (
	"context"
	"fmt"

	"github.com/kirillbaranov/figma-map/internal/storybook"
	"github.com/spf13/cobra"
)

func newScanCmd() *cobra.Command {
	var (
		storybookURL string
		project      string
		out          string
	)

	c := &cobra.Command{
		Use:   "scan",
		Short: "Screenshot Storybook stories into a code-component catalog",
		Long: "scan reads a running Storybook's index.json, screenshots each UI " +
			"story with headless Chrome, resolves each component's real import from " +
			"its story source, and writes catalog.json plus PNGs.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			if storybookURL == "" {
				storybookURL = cfg.Storybook
			}

			fmt.Printf("Fetching index.json from %s …\n", storybookURL)
			raw, err := storybook.FetchIndex(storybookURL)
			if err != nil {
				return err
			}

			stories, err := storybook.ParseIndex(raw)
			if err != nil {
				return err
			}
			if len(stories) == 0 {
				return fmt.Errorf("no UI/* stories found in index.json")
			}
			fmt.Printf("Found %d stories. Resolving imports …\n", len(stories))

			if err := storybook.ResolveImports(stories, project); err != nil {
				return err
			}

			fmt.Println("Capturing screenshots …")
			cap := storybook.NewCapturer(storybookURL)
			if err := cap.CaptureAll(context.Background(), stories, out); err != nil {
				return err
			}

			catalog := storybook.Catalog{Storybook: storybookURL, Stories: stories}
			if err := catalog.Save(out); err != nil {
				return err
			}

			fmt.Printf("Wrote %s/catalog.json (%d stories)\n", out, len(stories))
			return nil
		},
	}

	c.Flags().StringVar(&storybookURL, "storybook", "", "Storybook base URL (default from config)")
	c.Flags().StringVar(&project, "project", ".", "project root containing story source files (for import resolution)")
	c.Flags().StringVar(&out, "out", "catalog", "output catalog directory")
	return c
}
