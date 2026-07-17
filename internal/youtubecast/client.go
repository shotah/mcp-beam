package youtubecast

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go2tv.app/go2tv/v2/castprotocol"
	"go2tv.app/go2tv/v2/castprotocol/v2/application"
	"go2tv.app/go2tv/v2/castprotocol/v2/cast"
	pb "go2tv.app/go2tv/v2/castprotocol/v2/cast/proto"
)

const (
	// AppID is the official YouTube Cast receiver application id.
	AppID = "233637DE"
	// Namespace is the YouTube MDX cast namespace.
	Namespace = "urn:x-cast:com.google.youtube.mdx"

	typeGetScreenID = "getMdxSessionStatus"
	typeStatus      = "mdxSessionStatus"

	defaultSender = "sender-0"
	defaultRecv   = "receiver-0"
	namespaceRecv = "urn:x-cast:com.google.cast.receiver"

	screenIDTimeout = 12 * time.Second
	appReadyTimeout = 15 * time.Second
)

var videoIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

// Client launches the YouTube Cast app, resolves screenId via MDX, and
// plays a video through the lounge API. It also implements enough of the
// Chromecast control surface for mcp-beam session lifecycle (stop/volume/close).
type Client struct {
	app        *application.Application
	conn       cast.Conn
	host       string
	port       int
	httpClient *http.Client

	mu        sync.Mutex
	connected bool
	requestID atomic.Int32
}

// NewClient creates a YouTube cast client for deviceAddr (e.g. http://192.168.1.10:8009).
func NewClient(deviceAddr string) (*Client, error) {
	u, err := url.Parse(deviceAddr)
	if err != nil {
		return nil, fmt.Errorf("parse device addr: %w", err)
	}
	host := u.Hostname()
	port := 8009
	if u.Port() != "" {
		if _, err := fmt.Sscanf(u.Port(), "%d", &port); err != nil {
			return nil, fmt.Errorf("parse device port: %w", err)
		}
	}

	conn := cast.NewConnection()
	app := application.NewApplication(
		application.WithConnection(conn),
		application.WithConnectionRetries(5),
	)
	return &Client{
		app:        app,
		conn:       conn,
		host:       host,
		port:       port,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}, nil
}

// Connect establishes the Chromecast control channel.
func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.app.Start(c.host, c.port); err != nil {
		return fmt.Errorf("youtube cast connect: %w", err)
	}
	c.connected = true
	return nil
}

// PlayVideo launches the YouTube receiver and starts videoID.
func (c *Client) PlayVideo(ctx context.Context, videoID string, startSeconds int) error {
	videoID = strings.TrimSpace(videoID)
	if !ValidVideoID(videoID) {
		return fmt.Errorf("invalid youtube video id %q", videoID)
	}
	if startSeconds < 0 {
		return fmt.Errorf("start_seconds must be >= 0")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	c.mu.Lock()
	connected := c.connected
	c.mu.Unlock()
	if !connected {
		return fmt.Errorf("not connected")
	}

	if err := c.ensureYouTubeApp(ctx); err != nil {
		return err
	}
	screenID, err := c.fetchScreenID(ctx)
	if err != nil {
		return err
	}
	session := NewLoungeSession(screenID, c.httpClient)
	if err := session.PlayVideo(ctx, videoID, startSeconds); err != nil {
		return fmt.Errorf("youtube lounge play: %w", err)
	}
	return nil
}

func (c *Client) ensureYouTubeApp(ctx context.Context) error {
	if app := c.app.App(); app != nil && app.AppId == AppID && app.TransportId != "" {
		return nil
	}

	payload := &cast.LaunchRequest{
		PayloadHeader: cast.LaunchHeader,
		AppId:         AppID,
	}
	if err := c.send(payload, defaultSender, defaultRecv, namespaceRecv); err != nil {
		return fmt.Errorf("launch youtube app: %w", err)
	}

	deadline := time.Now().Add(appReadyTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}
		_ = c.app.Update()
		if app := c.app.App(); app != nil && app.AppId == AppID && app.TransportId != "" {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(400 * time.Millisecond):
		}
	}
	return fmt.Errorf("youtube app %s did not become ready", AppID)
}

func (c *Client) fetchScreenID(ctx context.Context) (string, error) {
	app := c.app.App()
	if app == nil || app.TransportId == "" {
		return "", fmt.Errorf("youtube transport id unavailable")
	}
	transportID := app.TransportId

	screenCh := make(chan string, 1)
	c.app.AddMessageFunc(func(msg *pb.CastMessage) {
		if msg == nil || msg.PayloadUtf8 == nil {
			return
		}
		if id := parseScreenID(*msg.PayloadUtf8); id != "" {
			select {
			case screenCh <- id:
			default:
			}
		}
	})

	payload := &mdxPayload{Type: typeGetScreenID}
	if err := c.send(payload, defaultSender, transportID, Namespace); err != nil {
		return "", fmt.Errorf("getMdxSessionStatus: %w", err)
	}

	timeout := screenIDTimeout
	if d, ok := ctx.Deadline(); ok {
		if remaining := time.Until(d); remaining > 0 && remaining < timeout {
			timeout = remaining
		}
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case id := <-screenCh:
		return id, nil
	case <-timer.C:
		return "", fmt.Errorf("timed out waiting for youtube screenId")
	}
}

func parseScreenID(payload string) string {
	var msg struct {
		Type string `json:"type"`
		Data struct {
			ScreenID string `json:"screenId"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(payload), &msg); err != nil {
		return ""
	}
	if msg.Type != typeStatus {
		return ""
	}
	return strings.TrimSpace(msg.Data.ScreenID)
}

type mdxPayload struct {
	Type      string `json:"type"`
	RequestId int    `json:"requestId,omitempty"`
}

func (p *mdxPayload) SetRequestId(id int) { p.RequestId = id }

func (c *Client) send(payload cast.Payload, sourceID, destinationID, namespace string) error {
	id := int(c.requestID.Add(1))
	payload.SetRequestId(id)
	return c.conn.Send(id, payload, sourceID, destinationID, namespace)
}

// ValidVideoID reports whether id looks like a YouTube video id.
func ValidVideoID(id string) bool {
	return videoIDPattern.MatchString(strings.TrimSpace(id))
}

// WatchURL returns the canonical youtube.com watch URL for videoID.
func WatchURL(videoID string) string {
	return "https://www.youtube.com/watch?v=" + strings.TrimSpace(videoID)
}

// --- adapters.CastClient compatibility for session lifecycle ---

func (c *Client) Load(mediaURL, contentType, title string, startTime int, duration float64, subtitleURL string, live bool) error {
	return fmt.Errorf("youtube cast client does not support Load; use PlayVideo / beam_youtube_video")
}

func (c *Client) LoadOnExisting(mediaURL, contentType, title string, startTime int, duration float64, subtitleURL string, live bool) error {
	return fmt.Errorf("youtube cast client does not support LoadOnExisting")
}

func (c *Client) Play() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.app.Unpause()
}

func (c *Client) Pause() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.app.Pause()
}

func (c *Client) Seek(seconds int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.app.SeekFromStart(seconds)
}

func (c *Client) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.app.Stop()
}

func (c *Client) SetVolume(level float32) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.app.SetVolume(level)
}

func (c *Client) SetMuted(muted bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.app.SetMuted(muted)
}

func (c *Client) GetStatus() (*castprotocol.CastStatus, error) {
	if err := c.app.Update(); err != nil {
		return nil, err
	}
	_, media, vol := c.app.Status()
	status := &castprotocol.CastStatus{}
	if vol != nil {
		status.Volume = float32(vol.Level)
		status.Muted = vol.Muted
	}
	if media != nil {
		status.PlayerState = media.PlayerState
		status.CurrentTime = media.CurrentTime
		if media.Media.Duration > 0 {
			status.Duration = media.Media.Duration
		}
		status.ContentType = media.Media.ContentType
		status.MediaTitle = media.Media.Metadata.Title
	} else {
		// YouTube MDX often has no Default Media Receiver MEDIA_STATUS.
		status.PlayerState = "PLAYING"
	}
	return status, nil
}

func (c *Client) Close(stopMedia bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connected = false
	return c.app.Close(stopMedia)
}
