package youtubecast

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidVideoID(t *testing.T) {
	if !ValidVideoID("dQw4w9WgXcQ") {
		t.Fatal("expected valid id")
	}
	if ValidVideoID("short") {
		t.Fatal("expected invalid id")
	}
	if ValidVideoID("https://youtu.be/dQw4w9WgXcQ") {
		t.Fatal("urls are not video ids")
	}
}

func TestParseScreenID(t *testing.T) {
	got := parseScreenID(`{"type":"mdxSessionStatus","data":{"screenId":"screen-123"}}`)
	if got != "screen-123" {
		t.Fatalf("got %q", got)
	}
	if parseScreenID(`{"type":"other"}`) != "" {
		t.Fatal("expected empty for other type")
	}
}

func TestLoungeSessionPlayVideo(t *testing.T) {
	var tokenHits, bindHits, playlistHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch {
		case strings.Contains(r.URL.Path, "get_lounge_token_batch"):
			tokenHits++
			if !strings.Contains(string(body), "screen_ids=screen-abc") {
				t.Errorf("unexpected token body: %s", body)
			}
			_, _ = w.Write([]byte(`{"screens":[{"loungeToken":"token-xyz"}]}`))
		case strings.Contains(r.URL.Path, "bc/bind"):
			if r.URL.Query().Get("SID") == "" {
				bindHits++
				_, _ = w.Write([]byte(`[[0,["c","sid-1","",8]],[1,["S","gsess-1"]]]`))
				return
			}
			playlistHits++
			if got := r.Header.Get(loungeIDHeader); got != "token-xyz" {
				t.Errorf("missing lounge header, got %q", got)
			}
			if rid := r.URL.Query().Get("RID"); rid != "0" {
				t.Errorf("first setPlaylist RID=%q, want 0", rid)
			}
			if !strings.Contains(string(body), "dQw4w9WgXcQ") {
				t.Errorf("playlist body missing video id: %s", body)
			}
			if !strings.Contains(string(body), "req0__sc=setPlaylist") {
				t.Errorf("playlist body missing prefixed command req0__sc: %s", body)
			}
			if strings.Contains(string(body), "__sc=setPlaylist") && !strings.Contains(string(body), "req0__sc=setPlaylist") {
				t.Errorf("playlist used bare __sc without req prefix: %s", body)
			}
			if !strings.Contains(string(body), "req0_videoId") {
				t.Errorf("playlist body missing req prefix: %s", body)
			}
			if !strings.Contains(string(body), "req0_currentTime=12") {
				t.Errorf("playlist body missing start time: %s", body)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[[]]`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	session := NewLoungeSession("screen-abc", srv.Client())
	session.baseURL = srv.URL + "/"

	if err := session.PlayVideo(context.Background(), "dQw4w9WgXcQ", 12); err != nil {
		t.Fatalf("PlayVideo: %v", err)
	}
	if tokenHits != 1 || bindHits != 1 || playlistHits != 1 {
		t.Fatalf("hits token=%d bind=%d playlist=%d", tokenHits, bindHits, playlistHits)
	}
}
