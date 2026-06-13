package service

import (
	"context"
	"fmt"
	"net/http"
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

	add(fmt.Sprintf("figma bridge (%s)", s.cfg.Bridge), s.src.Ping(ctx))
	add("headless chrome", findChrome())
	add(fmt.Sprintf("storybook (%s)", s.cfg.Storybook), pingStorybook(s.cfg.Storybook))

	if _, present := s.cfg.APIKey(); present {
		add(fmt.Sprintf("API key (%s)", s.cfg.LLM.APIKeyEnv), nil)
	} else {
		add(fmt.Sprintf("API key — set $%s", s.cfg.LLM.APIKeyEnv), fmt.Errorf("not set"))
	}
	return r
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
