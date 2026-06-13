package codegen

import (
	"testing"

	"github.com/kirillbaranov/figma-map/internal/binding"
)

func TestJSX(t *testing.T) {
	button := binding.Component{
		Import: "@/components/ui/button",
		Symbol: "Button",
		Props: map[string]binding.Prop{
			"variant": {Values: []string{"default", "secondary", "ghost"}},
			"size":    {Values: []string{"default", "sm", "lg"}},
		},
	}

	tests := []struct {
		name string
		el   Element
		want string
	}{
		{
			name: "props and children",
			el: Element{
				Component: button,
				Props:     map[string]string{"variant": "secondary", "size": "lg"},
				Children:  "Continue",
			},
			want: `import { Button } from "@/components/ui/button"

<Button size="lg" variant="secondary">Continue</Button>`,
		},
		{
			name: "children no props",
			el: Element{
				Component: button,
				Children:  "Click",
			},
			want: `import { Button } from "@/components/ui/button"

<Button>Click</Button>`,
		},
		{
			name: "self closing no props",
			el: Element{
				Component: button,
			},
			want: `import { Button } from "@/components/ui/button"

<Button />`,
		},
		{
			name: "empty values skipped",
			el: Element{
				Component: button,
				Props:     map[string]string{"variant": "", "size": "sm"},
				Children:  "Go",
			},
			want: `import { Button } from "@/components/ui/button"

<Button size="sm">Go</Button>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := JSX(tt.el)
			if got != tt.want {
				t.Errorf("JSX mismatch:\n got: %q\nwant: %q", got, tt.want)
			}
		})
	}
}
