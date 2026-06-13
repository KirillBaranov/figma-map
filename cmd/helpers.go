package cmd

import (
	"fmt"
	"os"

	"github.com/kirillbaranov/figma-map/internal/config"
	"github.com/kirillbaranov/figma-map/internal/figma"
	"github.com/kirillbaranov/figma-map/internal/llm"
)

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// resolveFileKey returns the file key to operate on: the explicit flag, then
// the config value, else the single connected file (erroring if ambiguous).
func resolveFileKey(flag string, cfg config.Config, src *figma.Bridge) (string, error) {
	if flag != "" {
		return flag, nil
	}
	if cfg.FileKey != "" {
		return cfg.FileKey, nil
	}
	files, err := src.Files()
	if err != nil {
		return "", err
	}
	switch len(files) {
	case 0:
		return "", fmt.Errorf("no Figma files connected to the bridge (open the file and run the plugin)")
	case 1:
		return files[0].FileKey, nil
	default:
		return "", fmt.Errorf("multiple files connected; pass --file <fileKey>")
	}
}

// newLLM builds a vision client from config, requiring an API key.
func newLLM(cfg config.Config) (*llm.Client, error) {
	key, ok := cfg.APIKey()
	if !ok {
		return nil, fmt.Errorf("API key not set; export $%s", cfg.LLM.APIKeyEnv)
	}
	return llm.New(llm.Options{
		APIKey:  key,
		BaseURL: cfg.LLM.BaseURL,
		Model:   cfg.LLM.Model,
	}), nil
}
