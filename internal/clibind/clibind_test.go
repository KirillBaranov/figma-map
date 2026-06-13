package clibind

import (
	"testing"

	"github.com/spf13/cobra"
)

type sample struct {
	// positional
	NodeID string `json:"nodeId" jsonschema:"the node id" cli:"arg"`
	// flags
	File    string  `json:"file" jsonschema:"file key"`
	Depth   int     `json:"depth" jsonschema:"nesting depth" default:"1"`
	Scale   float64 `json:"scale" default:"2"`
	Verbose bool    `json:"verbose"`
	private string  //nolint:unused // ignored (unexported)
}

func TestRegisterAndParse(t *testing.T) {
	var in sample
	cmd := &cobra.Command{Use: "x"}
	apply, err := Register(cmd, &in)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Defaults applied at registration.
	if in.Depth != 1 || in.Scale != 2 {
		t.Errorf("defaults not applied: depth=%d scale=%v", in.Depth, in.Scale)
	}

	// Simulate cobra parsing flags into the bound struct.
	cmd.SetArgs([]string{"55:1102", "--file", "abc", "--depth", "3", "--verbose"})
	cmd.RunE = func(_ *cobra.Command, args []string) error { return apply(args) }
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if in.NodeID != "55:1102" {
		t.Errorf("positional NodeID = %q", in.NodeID)
	}
	if in.File != "abc" {
		t.Errorf("flag File = %q", in.File)
	}
	if in.Depth != 3 {
		t.Errorf("flag Depth = %d", in.Depth)
	}
	if !in.Verbose {
		t.Errorf("flag Verbose = %v", in.Verbose)
	}
	if in.Scale != 2 {
		t.Errorf("default Scale changed unexpectedly = %v", in.Scale)
	}
}

func TestArgCountEnforced(t *testing.T) {
	var in sample
	cmd := &cobra.Command{Use: "x", SilenceErrors: true, SilenceUsage: true}
	apply, err := Register(cmd, &in)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	cmd.RunE = func(_ *cobra.Command, args []string) error { return apply(args) }
	cmd.SetArgs([]string{}) // missing the required positional
	if err := cmd.Execute(); err == nil {
		t.Error("expected error for missing positional arg")
	}
}

func TestRejectsNonStructPtr(t *testing.T) {
	if _, err := Register(&cobra.Command{}, sample{}); err == nil {
		t.Error("expected error for non-pointer")
	}
	x := 5
	if _, err := Register(&cobra.Command{}, &x); err == nil {
		t.Error("expected error for pointer-to-non-struct")
	}
}
