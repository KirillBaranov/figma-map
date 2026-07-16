package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

// CurrentSchemaVersion is the figma-map.yaml shape this build of figma-map
// expects. Bump it (and append a migration below) whenever a config field
// is renamed, restructured, or gains a required default that an existing
// hand-written file wouldn't have.
const CurrentSchemaVersion = 1

// migration upgrades a config from schema version `from` to `from+1`. It
// operates on the raw parsed YAML node rather than the typed Config struct
// so a hand-edited file's comments and any keys this version of figma-map
// doesn't recognize survive the rewrite.
type migration struct {
	from     int
	describe string
	apply    func(*yaml.Node) error
}

// migrations is empty today — the schema is greenfield as of
// CurrentSchemaVersion = 1, nothing has changed shape yet. The next
// breaking config change appends here instead of inventing a new mechanism.
var migrations = []migration{}

// Migrate brings the YAML file at path up to CurrentSchemaVersion in
// place, applying any pending migrations in order, and returns a
// human-readable description of each one applied (nil if the file is
// already current, or doesn't exist — a config file is optional, and
// config.Load's defaults cover a missing one).
func Migrate(path string) (applied []string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	if len(data) == 0 {
		return nil, nil
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return nil, nil // empty or non-mapping document — nothing to migrate
	}
	doc := root.Content[0]

	current := schemaVersionOf(doc)
	if current >= CurrentSchemaVersion {
		return nil, nil
	}

	for _, m := range migrations {
		if m.from < current {
			continue
		}
		if err := m.apply(doc); err != nil {
			return applied, fmt.Errorf("migrate schema %d -> %d (%s): %w", m.from, m.from+1, m.describe, err)
		}
		applied = append(applied, m.describe)
	}
	setSchemaVersion(doc, CurrentSchemaVersion)

	out, err := yaml.Marshal(&root)
	if err != nil {
		return applied, fmt.Errorf("marshal migrated config: %w", err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return applied, fmt.Errorf("write migrated config %s: %w", path, err)
	}
	return applied, nil
}

func schemaVersionOf(doc *yaml.Node) int {
	for i := 0; i+1 < len(doc.Content); i += 2 {
		if doc.Content[i].Value == "schemaVersion" {
			v, _ := strconv.Atoi(doc.Content[i+1].Value)
			return v
		}
	}
	return 0
}

func setSchemaVersion(doc *yaml.Node, version int) {
	valStr := strconv.Itoa(version)
	for i := 0; i+1 < len(doc.Content); i += 2 {
		if doc.Content[i].Value == "schemaVersion" {
			doc.Content[i+1].Value = valStr
			doc.Content[i+1].Tag = "!!int"
			return
		}
	}
	doc.Content = append(doc.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: "schemaVersion"},
		&yaml.Node{Kind: yaml.ScalarNode, Value: valStr, Tag: "!!int"},
	)
}
