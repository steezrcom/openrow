package flexi

import (
	"context"
	"testing"
)

func TestFetcherWrapsClient(t *testing.T) {
	srv := newFixtureServer(t)
	defer srv.Close()

	f, err := NewFetcher(map[string]string{
		"base_url": srv.URL, "company": "demo", "username": "user1", "password": "pw",
	}, true)
	if err != nil {
		t.Fatalf("new fetcher: %v", err)
	}
	v, err := f.Connect(context.Background())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	_ = v
	evs, err := f.ListEvidences(context.Background())
	if err != nil || len(evs) != 3 {
		t.Fatalf("evidences: %+v err=%v", evs, err)
	}
	props, err := f.GetProperties(context.Background(), "adresar")
	if err != nil || len(props) != 5 {
		t.Fatalf("props: %+v err=%v", props, err)
	}
	if props[0].Name != "nazev" || props[0].Type != "string" {
		t.Fatalf("first property: %+v", props[0])
	}
}
