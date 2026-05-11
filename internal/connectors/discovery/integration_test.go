package discovery_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/openrow/openrow/internal/connectors/catalog/flexi"
	"github.com/openrow/openrow/internal/connectors/discovery"
)

type stubLLM struct{ resp string }

func (s stubLLM) Classify(ctx context.Context, prompt string) (string, error) { return s.resp, nil }

func TestDiscoveryEndToEnd(t *testing.T) {
	srv := newFlexiFixture(t)
	defer srv.Close()

	fetcher, err := flexi.NewFetcher(map[string]string{
		"base_url": srv.URL, "company": "demo", "username": "u", "password": "p",
	}, true)
	if err != nil {
		t.Fatalf("fetcher: %v", err)
	}
	svc := &discovery.Service{
		Fetcher:    fetcher,
		Heuristic:  flexi.NewHeuristic(),
		Classifier: stubLLM{resp: `{"role":"unknown","confidence":0.4}`},
	}
	art, items, err := svc.Run(context.Background(),
		&discovery.Binding{ConnectorID: flexi.ConnectorID, LLMClassification: true})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if art.Evidences["adresar"].Role != discovery.RoleCustomer {
		t.Fatalf("adresar should be customer, got %q", art.Evidences["adresar"].Role)
	}
	if art.Evidences["adresar"].OpenRowEntity == nil {
		t.Fatalf("customer should be promoted-proposed")
	}
	if len(items) == 0 {
		t.Fatalf("expected at least one review item")
	}
}

func newFlexiFixture(t *testing.T) *httptest.Server {
	t.Helper()
	dir := "../catalog/flexi/testdata"
	mux := http.NewServeMux()
	mux.HandleFunc("/c/demo/evidence-list.json", func(w http.ResponseWriter, r *http.Request) {
		serveFile(w, dir+"/evidence-list.json")
	})
	mux.HandleFunc("/c/demo/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/c/demo/adresar/properties.json":
			serveFile(w, dir+"/adresar-properties.json")
		case "/c/demo/adresar.json":
			serveFile(w, dir+"/adresar-samples.json")
		case "/c/demo/adresar/$count":
			_, _ = w.Write([]byte(`{"winstrom":{"@rowCount":"2"}}`))
		case "/c/demo/faktura-vydana/properties.json":
			_, _ = w.Write([]byte(`{"properties":{"property":[{"propertyName":"kod","type":"string"}]}}`))
		case "/c/demo/faktura-vydana.json":
			_, _ = w.Write([]byte(`{"winstrom":{"faktura-vydana":[]}}`))
		case "/c/demo/faktura-vydana/$count":
			_, _ = w.Write([]byte(`{"winstrom":{"@rowCount":"0"}}`))
		case "/c/demo/cenik/properties.json":
			_, _ = w.Write([]byte(`{"properties":{"property":[{"propertyName":"nazev","type":"string"}]}}`))
		case "/c/demo/cenik.json":
			_, _ = w.Write([]byte(`{"winstrom":{"cenik":[]}}`))
		case "/c/demo/cenik/$count":
			_, _ = w.Write([]byte(`{"winstrom":{"@rowCount":"0"}}`))
		default:
			http.NotFound(w, r)
		}
	})
	return httptest.NewServer(mux)
}

func serveFile(w http.ResponseWriter, path string) {
	b, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
}
