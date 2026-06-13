package figma

import (
	"encoding/json"
	"testing"
)

// fixture mirrors the bridge serializer output for a frame + a text child,
// including the number|"mixed" union the decoder must tolerate.
const styleFixture = `{
  "id": "1:1", "name": "Card", "type": "FRAME",
  "bounds": {"x":0,"y":0,"width":350,"height":200},
  "styles": {
    "opacity": 1,
    "fills": [{"type":"SOLID","color":"#ffffff","opacity":1}],
    "cornerRadius": 8,
    "strokeWeight": "mixed",
    "autoLayout": {"direction":"VERTICAL","gap":16,"primaryAxisAlign":"MIN","counterAxisAlign":"CENTER"},
    "padding": {"top":32,"right":32,"bottom":64,"left":32}
  },
  "children": [
    {"id":"1:2","name":"Title","type":"TEXT","characters":"Hello",
     "bounds":{"x":32,"y":32,"width":286,"height":24},
     "styles":{
       "fills":[{"type":"SOLID","color":"#18181b","opacity":1}],
       "fontSize":24,"fontFamily":"Inter","fontWeight":600,
       "lineHeight":{"unit":"PIXELS","value":32},
       "textAlignHorizontal":"LEFT"
     }}
  ]
}`

func TestStyleDecode(t *testing.T) {
	var n Node
	if err := json.Unmarshal([]byte(styleFixture), &n); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if n.Styles == nil {
		t.Fatal("frame styles not decoded")
	}
	// Color
	if len(n.Styles.Fills) != 1 || n.Styles.Fills[0].Color != "#ffffff" {
		t.Errorf("fills = %+v", n.Styles.Fills)
	}
	// Number token
	if !n.Styles.CornerRadius.Set || n.Styles.CornerRadius.Value != 8 {
		t.Errorf("cornerRadius = %+v", n.Styles.CornerRadius)
	}
	// number|"mixed" union must not break decoding
	if !n.Styles.StrokeWeight.Mixed {
		t.Errorf("strokeWeight should be mixed, got %+v", n.Styles.StrokeWeight)
	}
	// Auto-layout
	if n.Styles.AutoLayout == nil || n.Styles.AutoLayout.Direction != "VERTICAL" || n.Styles.AutoLayout.Gap != 16 {
		t.Errorf("autoLayout = %+v", n.Styles.AutoLayout)
	}
	// Padding
	if n.Styles.Padding == nil || n.Styles.Padding.Bottom != 64 {
		t.Errorf("padding = %+v", n.Styles.Padding)
	}

	// Text child typography
	if len(n.Children) != 1 {
		t.Fatalf("want 1 child, got %d", len(n.Children))
	}
	txt := n.Children[0].Styles
	if txt == nil || !txt.FontSize.Set || txt.FontSize.Value != 24 {
		t.Errorf("text fontSize = %+v", txt)
	}
	if txt.FontFamily != "Inter" || !txt.FontWeight.Set || txt.FontWeight.Value != 600 {
		t.Errorf("text font = %+v", txt)
	}
	if txt.LineHeight == nil || txt.LineHeight.Unit != "PIXELS" || txt.LineHeight.Value != 32 {
		t.Errorf("lineHeight = %+v", txt.LineHeight)
	}
}

func TestMaybeNumRoundTrip(t *testing.T) {
	cases := map[string]struct {
		mixed bool
		set   bool
		val   float64
	}{
		"8":       {false, true, 8},
		`"mixed"`: {true, true, 0},
		"null":    {false, false, 0},
	}
	for in, want := range cases {
		var m MaybeNum
		if err := json.Unmarshal([]byte(in), &m); err != nil {
			t.Fatalf("unmarshal %s: %v", in, err)
		}
		if m.Mixed != want.mixed || m.Set != want.set || m.Value != want.val {
			t.Errorf("%s → %+v, want %+v", in, m, want)
		}
	}
}
