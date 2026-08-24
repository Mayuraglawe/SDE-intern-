package order

import (
	"net/http"
	"time"
)

// Config holds runtime options for streaming order updates.
type Config struct {
	CSVFilePath  string
	TargetURL    string
	RateLimitMPS int // Messages per second (e.g., 50)
	HTTPClient   *http.Client
}

// StreamStats tracks processing metrics during streaming.
type StreamStats struct {
	TotalRows    int
	AcceptedRows int
	SkippedRows  int
	SentEvents   int
	HTTPFailures int
	StartTime    time.Time
	EndTime      time.Time
}
