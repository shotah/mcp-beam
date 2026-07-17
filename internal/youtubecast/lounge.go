package youtubecast

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultYouTubeBaseURL = "https://www.youtube.com/"
	loungeIDHeader        = "X-YouTube-LoungeId-Token"
	actionSetPlaylist     = "setPlaylist"
)

var (
	gsessionIDRegex = regexp.MustCompile(`"S","([^"]+)"`)
	sidRegex        = regexp.MustCompile(`"c","([^"]+)"`)
)

// LoungeSession drives YouTube MDX playback via the lounge HTTP API
// (same protocol used by casttube / pychromecast).
type LoungeSession struct {
	screenID    string
	baseURL     string
	httpClient  *http.Client
	mu          sync.Mutex
	loungeToken string
	gsessionID  string
	sid         string
	rid         int
	reqCount    int
}

func NewLoungeSession(screenID string, httpClient *http.Client) *LoungeSession {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &LoungeSession{
		screenID:   strings.TrimSpace(screenID),
		baseURL:    defaultYouTubeBaseURL,
		httpClient: httpClient,
	}
}

func (s *LoungeSession) bindURL() string {
	return strings.TrimRight(s.baseURL, "/") + "/api/lounge/bc/bind"
}

func (s *LoungeSession) loungeTokenURL() string {
	return strings.TrimRight(s.baseURL, "/") + "/api/lounge/pairing/get_lounge_token_batch"
}

// PlayVideo starts a fresh lounge session and plays videoID immediately.
func (s *LoungeSession) PlayVideo(ctx context.Context, videoID string, startSeconds int) error {
	videoID = strings.TrimSpace(videoID)
	if videoID == "" {
		return fmt.Errorf("video id is empty")
	}
	if startSeconds < 0 {
		startSeconds = 0
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.getLoungeToken(ctx); err != nil {
		return err
	}
	if err := s.bind(ctx); err != nil {
		return err
	}
	return s.initializeQueue(ctx, videoID, startSeconds)
}

func (s *LoungeSession) getLoungeToken(ctx context.Context) error {
	form := url.Values{"screen_ids": {s.screenID}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.loungeTokenURL(), strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("lounge token request: %w", err)
	}
	req.Header.Set("Origin", s.baseURL)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("lounge token: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("lounge token read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("lounge token: status %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	var parsed struct {
		Screens []struct {
			LoungeToken string `json:"loungeToken"`
		} `json:"screens"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("lounge token decode: %w", err)
	}
	if len(parsed.Screens) == 0 || strings.TrimSpace(parsed.Screens[0].LoungeToken) == "" {
		return fmt.Errorf("lounge token missing in response")
	}
	s.loungeToken = parsed.Screens[0].LoungeToken
	return nil
}

func (s *LoungeSession) bind(ctx context.Context) error {
	s.rid = 0
	s.reqCount = 0

	params := url.Values{
		"RID":  {"0"},
		"VER":  {"8"},
		"CVER": {"1"},
	}
	form := url.Values{
		"device":       {"REMOTE_CONTROL"},
		"id":           {"mcpbeamytcastaaaaaaaaaaaa"},
		"name":         {"mcp-beam"},
		"mdx-version":  {"3"},
		"pairing_type": {"cast"},
		"app":          {"android-phone-13.14.55"},
	}

	body, err := s.post(ctx, s.bindURL(), params, form, false)
	if err != nil {
		return fmt.Errorf("lounge bind: %w", err)
	}

	sid := sidRegex.FindStringSubmatch(body)
	gsession := gsessionIDRegex.FindStringSubmatch(body)
	if len(sid) < 2 || len(gsession) < 2 {
		return fmt.Errorf("lounge bind: missing sid/gsessionid in response: %s", truncate(body, 200))
	}
	s.sid = sid[1]
	s.gsessionID = gsession[1]
	// Keep rid at 0 for the first session command (matches casttube/pychromecast).
	return nil
}

func (s *LoungeSession) initializeQueue(ctx context.Context, videoID string, startSeconds int) error {
	// casttube prefixes keys that start with "_" (including "__sc") with reqN.
	// So the command field must be "req0__sc", not bare "__sc".
	prefix := fmt.Sprintf("req%d", s.reqCount)
	form := url.Values{
		"count":                  {"1"},
		prefix + "__sc":          {actionSetPlaylist},
		prefix + "_listId":       {""},
		prefix + "_currentTime":  {strconv.Itoa(startSeconds)},
		prefix + "_currentIndex": {"-1"},
		prefix + "_audioOnly":    {"false"},
		prefix + "_videoId":      {videoID},
	}
	params := url.Values{
		"SID":        {s.sid},
		"gsessionid": {s.gsessionID},
		"RID":        {strconv.Itoa(s.rid)},
		"VER":        {"8"},
		"CVER":       {"1"},
	}
	if _, err := s.post(ctx, s.bindURL(), params, form, true); err != nil {
		return fmt.Errorf("lounge setPlaylist: %w", err)
	}
	return nil
}

func (s *LoungeSession) post(ctx context.Context, endpoint string, params, form url.Values, sessionRequest bool) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Origin", s.baseURL)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if s.loungeToken != "" {
		req.Header.Set(loungeIDHeader, s.loungeToken)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	if sessionRequest {
		s.reqCount++
		s.rid++
	}
	return string(body), nil
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
