package config

import (
	"bufio"
	"os"
	"strings"
)

// LoadEnvFile reads simple KEY=VALUE lines from path (typically ".env") and
// sets them via os.Setenv — but only for keys not already present in the
// environment, so an explicit `export FOO=bar` before running the tool always
// wins over the file. A missing file is not an error: most setups have no
// .env and rely on the shell environment alone.
func LoadEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if key == "" {
			continue
		}
		if _, set := os.LookupEnv(key); set {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	return scanner.Err()
}
