package figma

import "testing"

func TestTopLevelFrames(t *testing.T) {
	doc := &Node{
		Type: "PAGE",
		Children: []Node{
			{ID: "1", Type: "FRAME", Name: "Button"},
			{ID: "2", Type: "TEXT", Name: "heading"},
			{ID: "3", Type: "FRAME", Name: "Input"},
		},
	}
	frames := doc.TopLevelFrames()
	if len(frames) != 2 {
		t.Fatalf("expected 2 frames, got %d", len(frames))
	}
	if frames[0].Name != "Button" || frames[1].Name != "Input" {
		t.Errorf("unexpected frames: %+v", frames)
	}
}

func TestFirstText(t *testing.T) {
	n := Node{
		Type: "INSTANCE",
		Children: []Node{
			{Type: "FRAME", Children: []Node{
				{Type: "TEXT", Characters: "Continue"},
			}},
		},
	}
	if got := n.FirstText(); got != "Continue" {
		t.Errorf("FirstText() = %q, want Continue", got)
	}

	empty := Node{Type: "FRAME"}
	if got := empty.FirstText(); got != "" {
		t.Errorf("FirstText() on empty = %q, want empty", got)
	}
}

func TestDecodeSingleNode(t *testing.T) {
	// Object form
	if n, err := decodeSingleNode([]byte(`{"id":"1","name":"button","type":"INSTANCE"}`)); err != nil || n.ID != "1" {
		t.Errorf("object form failed: node=%+v err=%v", n, err)
	}
	// Array form
	if n, err := decodeSingleNode([]byte(`[{"id":"2","name":"x","type":"FRAME"}]`)); err != nil || n.ID != "2" {
		t.Errorf("array form failed: node=%+v err=%v", n, err)
	}
}
