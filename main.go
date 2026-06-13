// Command figma-map maps Figma design components to code components using a
// vision LLM, producing a reviewable binding and deterministic codegen.
package main

import (
	"fmt"
	"os"

	"github.com/kirillbaranov/figma-map/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
