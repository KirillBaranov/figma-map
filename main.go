// Command figma-map maps Figma design components to code components using a
// vision LLM, producing a reviewable binding and deterministic codegen.
package main

import (
	"fmt"
	"os"

	"github.com/kirillbaranov/figma-map/cmd"
)

// Build information, injected via -ldflags at release time.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if err := cmd.Execute(cmd.BuildInfo{Version: version, Commit: commit, Date: date}, Assets); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
