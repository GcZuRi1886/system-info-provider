package types

// MediaInfo describes the currently active MPRIS media player
type MediaInfo struct {
	Active          bool    `json:"active"`           // false when no player is available
	Player          string  `json:"player,omitempty"` // e.g. "spotify", "firefox.instance_1_23"
	Status          string  `json:"status,omitempty"` // Playing, Paused, Stopped
	Title           string  `json:"title,omitempty"`
	Artist          string  `json:"artist,omitempty"`
	Album           string  `json:"album,omitempty"`
	ArtURL          string  `json:"art_url,omitempty"`
	Length          float64 `json:"length"`                     // in seconds
	Position        float64 `json:"position"`                   // in seconds, at PositionUpdated
	PositionUpdated int64   `json:"position_updated,omitempty"` // unix ms when Position was read
	CanGoNext       bool    `json:"can_go_next"`
	CanGoPrevious   bool    `json:"can_go_previous"`
	CanPlay         bool    `json:"can_play"`
	CanPause        bool    `json:"can_pause"`
	CanSeek         bool    `json:"can_seek"`
}
