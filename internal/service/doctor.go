package service

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"
)

// Check is one doctor probe result.
type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// Report is the result of doctor: per-check status and an overall verdict.
type Report struct {
	Checks []Check `json:"checks"`
	OK     bool    `json:"ok"`
}

// Doctor verifies the bridge, headless Chrome, Storybook, and API key. It is
// deterministic and never requires the key to be present (it only reports it).
func (s *Service) Doctor(ctx context.Context) Report {
	r := Report{OK: true}
	add := func(name string, err error) {
		c := Check{Name: name, OK: err == nil}
		if err != nil {
			c.Detail = err.Error()
			r.OK = false
		}
		r.Checks = append(r.Checks, c)
	}

	// The bridge process being up and a Figma plugin actually being connected
	// to it are two different failure modes that look identical to an agent
	// if only Ping is checked — split them into separate checks so "bridge
	// down" and "bridge up but no plugin connected" are distinguishable.
	bridgeErr := s.src.Ping(ctx)
	add(fmt.Sprintf("figma bridge (%s)", s.cfg.Bridge), bridgeErr)
	add("figma plugin connected", pluginConnected(ctx, s, bridgeErr))
	add("headless chrome", findChrome())
	add(fmt.Sprintf("storybook (%s)", s.cfg.Storybook), pingStorybook(s.cfg.Storybook))

	if _, present := s.cfg.APIKey(); present {
		add(fmt.Sprintf("API key (%s)", s.cfg.LLM.APIKeyEnv), nil)
	} else {
		add(fmt.Sprintf("API key — set $%s", s.cfg.LLM.APIKeyEnv), fmt.Errorf("not set"))
	}
	return r
}

// pluginConnected reports whether a Figma plugin is actually connected to
// the (already-reachable) bridge — the bridge process can be up with zero
// files connected, which otherwise looks identical to "bridge down" to an
// agent reading just the Ping check.
func pluginConnected(ctx context.Context, s *Service, bridgeErr error) error {
	if bridgeErr != nil {
		return fmt.Errorf("bridge unreachable — restart it: cd backend && node dist/index.js")
	}
	files, err := s.src.Files(ctx)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("bridge is up but no Figma file is connected — open the file " +
			"and run the plugin in Figma (Plugins → Development)")
	}
	return nil
}

// findChrome locates a Chrome/Chromium binary the way chromedp does.
func findChrome() error {
	candidates := []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome"}
	switch runtime.GOOS {
	case "darwin":
		candidates = append(candidates,
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		)
	case "windows":
		candidates = append(candidates, "chrome.exe")
		for _, envVar := range []string{"ProgramFiles", "ProgramFiles(x86)", "LocalAppData"} {
			base := os.Getenv(envVar)
			if base == "" {
				continue
			}
			candidates = append(candidates,
				base+`\Google\Chrome\Application\chrome.exe`,
				base+`\Chromium\Application\chrome.exe`,
			)
		}
	}
	for _, c := range candidates {
		if _, err := exec.LookPath(c); err == nil {
			return nil
		}
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
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("index.json returned %d", resp.StatusCode)
	}
	return nil
}
