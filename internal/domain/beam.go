package domain

type BeamRequest struct {
	Source        string `json:"source"`
	TargetDevice  string `json:"target_device"`
	Transcode     string `json:"transcode,omitempty"`
	SubtitlesPath string `json:"subtitles_path,omitempty"`
	StartSeconds  *int   `json:"start_seconds,omitempty"`
}

type BeamResult struct {
	OK          bool     `json:"ok"`
	SessionID   string   `json:"session_id"`
	DeviceID    string   `json:"device_id"`
	MediaURL    string   `json:"media_url"`
	Transcoding bool     `json:"transcoding"`
	Warnings    []string `json:"warnings"`
}

type StopRequest struct {
	TargetDevice string `json:"target_device,omitempty"`
	SessionID    string `json:"session_id,omitempty"`
}

type StopResult struct {
	OK               bool     `json:"ok"`
	StoppedSessionID string   `json:"stopped_session_id"`
	DeviceID         string   `json:"device_id"`
	Warnings         []string `json:"warnings,omitempty"`
}

type PlaybackControlRequest struct {
	TargetDevice string `json:"target_device,omitempty"`
	SessionID    string `json:"session_id,omitempty"`
}

type PlaybackControlResult struct {
	OK        bool   `json:"ok"`
	SessionID string `json:"session_id"`
	DeviceID  string `json:"device_id"`
	State     string `json:"state"`
}

type SeekRequest struct {
	TargetDevice    string   `json:"target_device,omitempty"`
	SessionID       string   `json:"session_id,omitempty"`
	PositionSeconds *int     `json:"position_seconds,omitempty"`
	PositionPercent *float64 `json:"position_percent,omitempty"`
	FromEndSeconds  *int     `json:"from_end_seconds,omitempty"`
	DeltaSeconds    *int     `json:"delta_seconds,omitempty"`
}

type SeekResult struct {
	OK                      bool     `json:"ok"`
	SessionID               string   `json:"session_id"`
	DeviceID                string   `json:"device_id"`
	PositionSeconds         int      `json:"position_seconds"`
	RequestedMode           string   `json:"requested_mode"`
	ResolvedPositionSeconds int      `json:"resolved_position_seconds"`
	DurationSeconds         *float64 `json:"duration_seconds,omitempty"`
}

type StatusRequest struct {
	TargetDevice string `json:"target_device,omitempty"`
	SessionID    string `json:"session_id,omitempty"`
}

type StatusResult struct {
	OK              bool     `json:"ok"`
	SessionID       string   `json:"session_id"`
	DeviceID        string   `json:"device_id"`
	DeviceName      string   `json:"device_name,omitempty"`
	Protocol        string   `json:"protocol"`
	State           string   `json:"state"`
	PositionSeconds *float64 `json:"position_seconds,omitempty"`
	DurationSeconds *float64 `json:"duration_seconds,omitempty"`
	Title           string   `json:"title,omitempty"`
	ContentType     string   `json:"content_type,omitempty"`
	MediaURL        string   `json:"media_url,omitempty"`
	Transcoding     bool     `json:"transcoding"`
	Warnings        []string `json:"warnings,omitempty"`
}

type ToolError struct {
	Code           string         `json:"code"`
	Message        string         `json:"message"`
	Limitations    []Limitation   `json:"limitations,omitempty"`
	SuggestedFixes []string       `json:"suggested_fixes,omitempty"`
	Details        map[string]any `json:"details,omitempty"`
}

func (e *ToolError) Error() string {
	if e == nil {
		return ""
	}
	return e.Code + ": " + e.Message
}
