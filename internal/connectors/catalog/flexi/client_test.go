package flexi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func newFixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/c/demo/evidence-list.json", func(w http.ResponseWriter, r *http.Request) {
		if user, _, _ := r.BasicAuth(); user != "user1" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		serve(w, "testdata/evidence-list.json")
	})
	mux.HandleFunc("/c/demo/adresar/properties.json", func(w http.ResponseWriter, r *http.Request) {
		serve(w, "testdata/adresar-properties.json")
	})
	mux.HandleFunc("/c/demo/adresar.json", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "limit=10") {
			http.Error(w, "missing limit", http.StatusBadRequest)
			return
		}
		serve(w, "testdata/adresar-samples.json")
	})
	mux.HandleFunc("/c/demo/adresar/$count", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"winstrom":{"@rowCount":"2"}}`))
	})
	return httptest.NewServer(mux)
}

func serve(w http.ResponseWriter, path string) {
	b, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
}

func TestClientListEvidences(t *testing.T) {
	srv := newFixtureServer(t)
	defer srv.Close()

	c, err := NewClient(map[string]string{
		"base_url": srv.URL,
		"company":  "demo",
		"username": "user1",
		"password": "pw",
	}, true)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	evs, err := c.ListEvidences(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(evs) != 3 || evs[0] != "adresar" {
		t.Fatalf("unexpected evidences: %+v", evs)
	}
}

func TestClientIntrospect(t *testing.T) {
	srv := newFixtureServer(t)
	defer srv.Close()

	c, _ := NewClient(map[string]string{
		"base_url": srv.URL, "company": "demo", "username": "user1", "password": "pw",
	}, true)
	props, err := c.GetProperties(context.Background(), "adresar")
	if err != nil {
		t.Fatalf("properties: %v", err)
	}
	if len(props) != 5 || props[0].Name != "nazev" {
		t.Fatalf("properties: %+v", props)
	}
	samples, err := c.GetSamples(context.Background(), "adresar", 10)
	if err != nil {
		t.Fatalf("samples: %v", err)
	}
	if len(samples) != 2 || samples[0]["nazev"] != "Acme s.r.o." {
		t.Fatalf("samples: %+v", samples)
	}
	count, err := c.Count(context.Background(), "adresar")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
}
