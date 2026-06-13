package cmd

import (
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"github.com/kirillbaranov/figma-map/internal/figma"
	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check that the bridge, Chrome, Storybook, and API key are available",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			ok := true
			check := func(name string, err error) {
				if err != nil {
					ok = false
					fmt.Printf("  ✗ %s — %v\n", name, err)
				} else {
					fmt.Printf("  ✓ %s\n", name)
				}
			}

			fmt.Println("figma-map doctor")

			// Bridge
			check(fmt.Sprintf("figma bridge (%s)", cfg.Bridge), figma.NewBridge(cfg.Bridge).Ping())

			// Chrome
			check("headless chrome", findChrome())

			// Storybook
			check(fmt.Sprintf("storybook (%s)", cfg.Storybook), pingStorybook(cfg.Storybook))

			// API key
			if _, present := cfg.APIKey(); present {
				fmt.Printf("  ✓ API key (%s)\n", cfg.LLM.APIKeyEnv)
			} else {
				ok = false
				fmt.Printf("  ✗ API key — set $%s\n", cfg.LLM.APIKeyEnv)
			}

			if !ok {
				return fmt.Errorf("one or more checks failed")
			}
			fmt.Println("all checks passed")
			return nil
		},
	}
}

// findChrome locates a Chrome/Chromium binary the way chromedp does.
func findChrome() error {
	candidates := []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome"}
	if runtime.GOOS == "darwin" {
		candidates = append(candidates,
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		)
	}
	for _, c := range candidates {
		if _, err := exec.LookPath(c); err == nil {
			return nil
		}
		// LookPath fails on absolute paths with spaces; stat directly.
		if fileExists(c) {
			return nil
		}
	}
	return fmt.Errorf("no Chrome/Chromium binary found")
}

func pingStorybook(url string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url + "/index.json")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("index.json returned %d", resp.StatusCode)
	}
	return nil
}
