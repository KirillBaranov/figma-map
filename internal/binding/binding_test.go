package binding

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	orig := Binding{
		Storybook: "http://localhost:6007",
		FigmaFile: "abc123",
		Components: map[string]Component{
			"Button": {
				FigmaNode: "13:1070",
				Import:    "@/components/ui/button",
				Symbol:    "Button",
				Props: map[string]Prop{
					"variant": {Values: []string{"default", "secondary"}},
					"size":    {Values: []string{"default", "lg"}},
				},
				Confidence: "high",
			},
		},
	}

	path := filepath.Join(t.TempDir(), "binding.yaml")
	if err := orig.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(orig, got) {
		t.Errorf("round trip mismatch:\n orig: %+v\n  got: %+v", orig, got)
	}
}

func TestPropDefault(t *testing.T) {
	if got := (Prop{Values: []string{"default", "lg"}}).Default(); got != "default" {
		t.Errorf("Default() = %q, want default", got)
	}
	if got := (Prop{}).Default(); got != "" {
		t.Errorf("empty Default() = %q, want empty", got)
	}
}

func TestComponentNamesSorted(t *testing.T) {
	b := Binding{Components: map[string]Component{
		"Input": {}, "Button": {}, "Card": {},
	}}
	got := b.ComponentNames()
	want := []string{"Button", "Card", "Input"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ComponentNames() = %v, want %v", got, want)
	}
}
