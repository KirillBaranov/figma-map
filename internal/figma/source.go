// Package figma defines the domain model for a Figma document and the Source
// interface that abstracts where that data comes from. The bridge backend
// (figma-mcp-bridge) is the only implementation today; a REST backend can be
// added later behind the same interface.
package figma

// Bounds is the absolute bounding box of a node, in Figma canvas coordinates.
type Bounds struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// Node is the source-independent representation of a Figma node. It carries
// only the fields figma-map needs; richer per-backend data is intentionally
// dropped at the boundary so downstream code stays backend-agnostic.
type Node struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Characters string `json:"characters,omitempty"`
	Bounds     Bounds `json:"bounds"`
	Styles     *Style `json:"styles,omitempty"`
	Children   []Node `json:"children,omitempty"`
}

// File identifies a Figma file connected to the source.
type File struct {
	FileKey  string `json:"fileKey"`
	FileName string `json:"fileName"`
}

// ScreenshotOpts controls how a node is rendered to an image.
type ScreenshotOpts struct {
	// Format is "PNG" (default), "JPG", or "SVG".
	Format string
	// Scale is the export scale factor; 0 means the backend default.
	Scale float64
}

// Source abstracts access to Figma data. Implementations are responsible for
// transport and for mapping their wire format into the domain Node model.
type Source interface {
	// Files lists the Figma files currently reachable through the source.
	Files() ([]File, error)
	// Document returns the current page's node tree for the given file.
	Document(fileKey string) (*Node, error)
	// Node returns a single node by id.
	Node(fileKey, id string) (*Node, error)
	// Selection returns the nodes currently selected in the editor.
	Selection(fileKey string) ([]Node, error)
	// Screenshot renders a node to image bytes.
	Screenshot(fileKey, id string, opts ScreenshotOpts) ([]byte, error)
}

// TopLevelFrames returns the direct FRAME children of the document root. On the
// shadcn Components page these are the per-component sections (e.g. "Button").
func (n *Node) TopLevelFrames() []Node {
	var frames []Node
	for _, c := range n.Children {
		if c.Type == "FRAME" {
			frames = append(frames, c)
		}
	}
	return frames
}

// FirstText walks the subtree depth-first and returns the first non-empty text
// content found, or "". Used to surface a label (e.g. a button's caption) as an
// extra signal for the vision model and for codegen children.
func (n *Node) FirstText() string {
	if n.Type == "TEXT" && n.Characters != "" {
		return n.Characters
	}
	for i := range n.Children {
		if t := n.Children[i].FirstText(); t != "" {
			return t
		}
	}
	return ""
}
