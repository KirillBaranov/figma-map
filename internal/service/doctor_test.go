package service

import (
	"context"
	"testing"

	"github.com/kirillbaranov/figma-map/internal/config"
	"github.com/kirillbaranov/figma-map/internal/figma"
)

// findCheck returns the named check from a Report, or fails the test.
func findCheck(t *testing.T, r Report, name string) Check {
	t.Helper()
	for _, c := range r.Checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no check named %q in %+v", name, r.Checks)
	return Check{}
}

// TestDoctor_PluginConnected covers Phase 10: "bridge up, no plugin
// connected" must be a distinct, non-ok check from "bridge down" — not
// silently look the same.
func TestDoctor_PluginConnected(t *testing.T) {
	// Bridge reachable (Ping ok via fakeSource), but zero files connected.
	fake := &fakeSource{}
	s := &Service{cfg: config.Config{}, src: fake}
	r := s.Doctor(context.Background())

	bridgeCheck := findCheck(t, r, "figma bridge ()")
	if !bridgeCheck.OK {
		t.Errorf("bridge check should be OK (Ping succeeds), got %+v", bridgeCheck)
	}
	pluginCheck := findCheck(t, r, "figma plugin connected")
	if pluginCheck.OK {
		t.Error("plugin-connected check should fail when no files are connected")
	}

	// Now a file is connected — both checks pass.
	fake.files = []figma.File{{FileKey: "k", FileName: "F"}}
	r2 := s.Doctor(context.Background())
	if !findCheck(t, r2, "figma plugin connected").OK {
		t.Error("plugin-connected check should pass once a file is connected")
	}
}

// TestDoctor_DormantIsNotAFailure covers Phase F: a dormant file (Figma tab
// backgrounded/throttled — requests going without real progress) is an
// honest heads-up, not an error — the check must still pass, just with an
// informational Detail an agent can read before a request stalls on it.
func TestDoctor_DormantIsNotAFailure(t *testing.T) {
	fake := &fakeSource{files: []figma.File{{FileKey: "k", FileName: "F", Status: "dormant"}}}
	s := &Service{cfg: config.Config{}, src: fake}
	r := s.Doctor(context.Background())

	check := findCheck(t, r, "figma plugin connected")
	if !check.OK {
		t.Errorf("dormant should not fail the check, got %+v", check)
	}
	if check.Detail == "" {
		t.Error("dormant should still surface an informational detail")
	}
}
