// Package config loads figma-map configuration from a YAML file with
// environment-variable overrides. Secrets never live in the file — the API key
// is read from the environment so a config can be safely committed.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config holds all tunable settings for the tool. Fields map 1:1 to keys in
// figma-map.yaml; the API key is intentionally absent and sourced from the
// environment via APIKey().
type Config struct {
	// SchemaVersion tracks which figma-map.yaml shape this file was written
	// against. Absent/0 means "written before schema versioning existed" —
	// `figma-map update` uses this to decide whether any migration in
	// internal/config/migrate.go needs to run. Not meant to be hand-edited.
	SchemaVersion int `yaml:"schemaVersion,omitempty"`

	// Bridge is the base URL of the running figma-bridge HTTP RPC server.
	Bridge string `yaml:"bridge"`
	// BridgeRepo is the path to a figma-map source checkout containing
	// backend/, for contributors building the backend from source instead
	// of `bridge up`'s default: fetching a prebuilt bundle matching the
	// running CLI's version. Optional and rarely needed — every other
	// operation just talks to whatever's already listening on Bridge.
	BridgeRepo string `yaml:"bridgeRepo"`
	// Storybook is the base URL of a running Storybook instance.
	Storybook string `yaml:"storybook"`
	// FileKey is the default Figma file key to operate on.
	FileKey string `yaml:"fileKey"`

	// LLM configuration. BaseURL is OpenAI-compatible, so it also points at the
	// kb-labs gateway or a local Ollama/llava server.
	LLM LLMConfig `yaml:"llm"`

	// Figma selects and configures the figma.Source backend (ADR-0003 §5).
	Figma FigmaSourceConfig `yaml:"figma"`

	// Codegen configures the `codegen` operation.
	Codegen CodegenConfig `yaml:"codegen"`
}

// CodegenConfig configures the `codegen` operation.
type CodegenConfig struct {
	// Target is the default output renderer ("jsx", "html", ...) used when
	// the caller doesn't pass --target explicitly. Defaults() sets this to
	// "jsx"; an explicit --target flag still overrides it (see
	// service.resolveTarget). A blank value in a hand-edited config also
	// falls back to "jsx" as a safety net.
	Target string `yaml:"target"`
}

// FigmaSourceConfig selects which figma.Source implementation to use.
type FigmaSourceConfig struct {
	// Source is "bridge" (default — live plugin via WebSocket, full feature
	// set including capture issues/compare) or "rest" (Figma REST API,
	// headless/CI-friendly, read-only — see ADR-0003 §5 and README's
	// Limitations for what "rest" can't do).
	Source string `yaml:"source"`
	// TokenEnv names the environment variable holding a Figma personal
	// access token / Dev Mode token, read only when Source is "rest".
	TokenEnv string `yaml:"tokenEnv"`
}

// LLMConfig configures the vision model used for matching and prop inference.
type LLMConfig struct {
	// BaseURL overrides the OpenAI API endpoint. Empty means the default
	// OpenAI endpoint.
	BaseURL string `yaml:"baseURL"`
	// Model is the vision-capable chat model id.
	Model string `yaml:"model"`
	// APIKeyEnv names the environment variable holding the API key.
	APIKeyEnv string `yaml:"apiKeyEnv"`
}

// Defaults returns a Config populated with sensible local-development values.
func Defaults() Config {
	return Config{
		Bridge:    "http://localhost:1994",
		Storybook: "http://localhost:6007",
		LLM: LLMConfig{
			Model:     "gpt-4o-mini",
			APIKeyEnv: "OPENAI_API_KEY",
		},
		Figma: FigmaSourceConfig{
			Source:   "bridge",
			TokenEnv: "FIGMA_TOKEN",
		},
		Codegen: CodegenConfig{
			Target: "jsx",
		},
	}
}

// Load reads config from path, layering it over Defaults(). A missing file is
// not an error — defaults are returned so the tool works out of the box.
func Load(path string) (Config, error) {
	cfg := Defaults()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg, nil
}

// APIKey returns the LLM API key from the configured environment variable.
// The bool is false when the variable is unset or empty.
func (c Config) APIKey() (string, bool) {
	env := c.LLM.APIKeyEnv
	if env == "" {
		env = "OPENAI_API_KEY"
	}
	key := os.Getenv(env)
	return key, key != ""
}

// FigmaToken returns the Figma REST API token from the configured
// environment variable (only meaningful when Figma.Source is "rest"). The
// bool is false when the variable is unset or empty.
func (c Config) FigmaToken() (string, bool) {
	env := c.Figma.TokenEnv
	if env == "" {
		env = "FIGMA_TOKEN"
	}
	token := os.Getenv(env)
	return token, token != ""
}
