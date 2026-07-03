package figma

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestRESTSource(t *testing.T, handler http.HandlerFunc) (*RESTSource, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	src := NewRESTSource("test-token", "file1")
	src.baseURL = srv.URL
	return src, srv
}

func TestRESTSource_Ping(t *testing.T) {
	src, _ := newTestRESTSource(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Figma-Token"); got != "test-token" {
			t.Errorf("X-Figma-Token = %q, want test-token", got)
		}
		_, _ = w.Write([]byte(`{"name":"My File"}`))
	})
	if err := src.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestRESTSource_Ping_NoFileKey(t *testing.T) {
	src := NewRESTSource("test-token", "")
	if err := src.Ping(context.Background()); err == nil {
		t.Fatal("expected error when no fileKey configured")
	}
}

func TestRESTSource_Document(t *testing.T) {
	src, _ := newTestRESTSource(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/files/file1" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{
			"document": {
				"id": "0:0", "name": "Document", "type": "DOCUMENT",
				"children": [
					{"id": "1:1", "name": "Page 1", "type": "CANVAS", "children": [
						{"id": "1:2", "name": "Button", "type": "FRAME",
						 "absoluteBoundingBox": {"x":1,"y":2,"width":100,"height":40},
						 "cornerRadius": 8,
						 "fills": [{"type":"SOLID","color":{"r":0.1,"g":0.2,"b":0.3,"a":1}}]}
					]}
				]
			}
		}`))
	})
	doc, err := src.Document(context.Background(), "file1")
	if err != nil {
		t.Fatalf("Document: %v", err)
	}
	if doc.ID != "0:0" || len(doc.Children) != 1 {
		t.Fatalf("unexpected document: %+v", doc)
	}
	button := doc.Children[0].Children[0]
	if button.Name != "Button" || button.Bounds.Width != 100 {
		t.Fatalf("unexpected button node: %+v", button)
	}
	if button.Styles == nil || !button.Styles.CornerRadius.Set || button.Styles.CornerRadius.Value != 8 {
		t.Fatalf("expected cornerRadius mapped, got %+v", button.Styles)
	}
	if got := FirstSolidCSS(button.Styles.Fills.Paints); got != "#1a334d" {
		t.Errorf("fill color = %q, want #1a334d", got)
	}
}

func TestRESTSource_Node_ComponentProperties(t *testing.T) {
	src, _ := newTestRESTSource(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/files/file1/nodes" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("ids"); got != "1:2" {
			t.Fatalf("ids query = %q, want 1:2", got)
		}
		_, _ = w.Write([]byte(`{
			"nodes": {
				"1:2": {"document": {
					"id": "1:2", "name": "Button", "type": "INSTANCE",
					"componentId": "9:9",
					"componentProperties": {"Size": {"type":"VARIANT","value":"Large"}}
				}}
			},
			"components": {"9:9": {"name": "Button", "componentSetId": "8:8"}},
			"componentSets": {"8:8": {"name": "Button Set"}}
		}`))
	})
	node, err := src.Node(context.Background(), "file1", "1:2")
	if err != nil {
		t.Fatalf("Node: %v", err)
	}
	if node.ComponentProps["Size"] != "Large" {
		t.Fatalf("componentProps not mapped: %+v", node.ComponentProps)
	}
}

func TestRESTSource_MainComponentName(t *testing.T) {
	src, _ := newTestRESTSource(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"nodes": {"1:2": {"document": {"id":"1:2","name":"Button","type":"INSTANCE","componentId":"9:9"}}},
			"components": {"9:9": {"name": "Button", "componentSetId": "8:8"}},
			"componentSets": {"8:8": {"name": "Button Set"}}
		}`))
	})
	name, err := src.MainComponentName(context.Background(), "file1", "1:2")
	if err != nil {
		t.Fatalf("MainComponentName: %v", err)
	}
	if name != "Button Set" {
		t.Fatalf("MainComponentName = %q, want Button Set", name)
	}
}

func TestRESTSource_Selection_NotSupported(t *testing.T) {
	src := NewRESTSource("test-token", "file1")
	_, err := src.Selection(context.Background(), "file1")
	if err == nil || !strings.Contains(err.Error(), "not available") {
		t.Fatalf("expected 'not available' error, got %v", err)
	}
}

func TestRESTSource_Screenshot(t *testing.T) {
	var srvURL string
	src, srv := newTestRESTSource(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/images/file1":
			_, _ = w.Write([]byte(`{"images":{"1:2":"` + srvURL + `/img/1_2.png"}}`))
		case "/img/1_2.png":
			_, _ = w.Write([]byte("fake-png-bytes"))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	})
	srvURL = srv.URL
	data, err := src.Screenshot(context.Background(), "file1", "1:2", ScreenshotOpts{})
	if err != nil {
		t.Fatalf("Screenshot: %v", err)
	}
	if string(data) != "fake-png-bytes" {
		t.Fatalf("Screenshot data = %q", data)
	}
}

func TestRESTSource_Metadata(t *testing.T) {
	src, _ := newTestRESTSource(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"name": "My File",
			"document": {"children": [
				{"id":"1:1","name":"Page 1","type":"CANVAS"},
				{"id":"1:2","name":"Page 2","type":"CANVAS"}
			]}
		}`))
	})
	meta, err := src.Metadata(context.Background(), "file1")
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if meta.FileName != "My File" || len(meta.Pages) != 2 {
		t.Fatalf("unexpected metadata: %+v", meta)
	}
}

func TestRESTSource_FindNodes(t *testing.T) {
	src, _ := newTestRESTSource(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"document": {"id":"0:0","name":"Document","type":"DOCUMENT","children":[
				{"id":"1:1","name":"Page 1","type":"CANVAS","children":[
					{"id":"1:2","name":"Primary Button","type":"INSTANCE"},
					{"id":"1:3","name":"Secondary Button","type":"INSTANCE"},
					{"id":"1:4","name":"Header","type":"TEXT","characters":"Welcome"}
				]}
			]}
		}`))
	})
	matches, err := src.FindNodes(context.Background(), "file1", FindNodesOptions{Query: "button"})
	if err != nil {
		t.Fatalf("FindNodes: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d: %+v", len(matches), matches)
	}
}

func TestRESTSource_NotAnIssueSource(t *testing.T) {
	var src Source = NewRESTSource("test-token", "file1")
	if _, ok := src.(IssueSource); ok {
		t.Fatal("RESTSource must not implement IssueSource (ADR-0003 §5)")
	}
}
