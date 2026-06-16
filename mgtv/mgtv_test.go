package mgtv

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func newTestClient(baseURL, apiURL string) *Client {
	cfg := DefaultConfig()
	cfg.BaseURL = baseURL
	cfg.APIURL = apiURL
	cfg.SearchURL = baseURL
	cfg.Rate = 0
	cfg.Retries = 0
	return NewClientConfig(cfg)
}

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("load fixture %s: %v", name, err)
	}
	return b
}

func TestParseClipIDs(t *testing.T) {
	fixture := loadFixture(t, "category_show.html")
	ids := parseClipIDs(fixture)
	if len(ids) == 0 {
		t.Fatal("expected at least one clip ID")
	}
	found := false
	for _, id := range ids {
		if id == "867784" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected clip ID 867784 in %v", ids)
	}
}

func TestFetchVideoInfo(t *testing.T) {
	fixture := loadFixture(t, "video_info.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("no User-Agent header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, srv.URL)
	v, err := c.fetchVideoInfo(context.Background(), "867784", "24423183")
	if err != nil {
		t.Fatal(err)
	}
	if v.Title != "爸爸当家 第五季" {
		t.Errorf("title = %q, want %q", v.Title, "爸爸当家 第五季")
	}
	if v.ClipID != "867784" {
		t.Errorf("clip_id = %q, want %q", v.ClipID, "867784")
	}
	if v.Type != "综艺" {
		t.Errorf("type = %q, want %q", v.Type, "综艺")
	}
	if v.Area != "内地" {
		t.Errorf("area = %q, want %q", v.Area, "内地")
	}
	if v.Language != "普通话" {
		t.Errorf("language = %q, want %q", v.Language, "普通话")
	}
	if v.ReleaseDate != "2026-05-18" {
		t.Errorf("release_date = %q, want %q", v.ReleaseDate, "2026-05-18")
	}
	if !strings.Contains(v.URL, "867784") {
		t.Errorf("url = %q, should contain clip ID", v.URL)
	}
}

func TestFetchCategoryClipIDs(t *testing.T) {
	fixture := loadFixture(t, "category_show.html")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(fixture)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, srv.URL)
	ids, err := c.fetchCategoryClipIDs(context.Background(), srv.URL+"/show/")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) == 0 {
		t.Fatal("expected at least one clip ID")
	}
}

func TestGetVideoNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, srv.URL)
	_, err := c.GetVideo(context.Background(), "999999")
	if err != ErrNotFound {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestParseVideoInfo(t *testing.T) {
	fixture := loadFixture(t, "video_info.json")
	v, err := parseVideoInfo(fixture, "867784", "24423183")
	if err != nil {
		t.Fatal(err)
	}
	if v.Description == "" {
		t.Error("description should not be empty")
	}
	if v.Genres == "" {
		t.Error("genres should not be empty")
	}
}
