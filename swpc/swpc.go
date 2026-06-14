// Package swpc is the library behind the swpc command line:
// the HTTP client, request shaping, and the typed data models for NOAA SWPC.
//
// The Client here is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries the
// transient failures (429 and 5xx) that any public site throws under load.
package swpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"
)

// Host is the NOAA SWPC API hostname.
const Host = "services.swpc.noaa.gov"

// Config holds the client configuration.
type Config struct {
	BaseURL   string
	UserAgent string
	Rate      time.Duration
	Retries   int
	Timeout   time.Duration
}

// DefaultConfig returns sensible defaults for the SWPC client.
func DefaultConfig() Config {
	return Config{
		BaseURL:   "https://services.swpc.noaa.gov",
		UserAgent: "tamnd-swpc-cli/0.1 (tamnd87@gmail.com)",
		Rate:      200 * time.Millisecond,
		Retries:   3,
		Timeout:   15 * time.Second,
	}
}

// Client talks to the NOAA SWPC API over HTTP.
type Client struct {
	HTTP      *http.Client
	UserAgent string
	BaseURL   string
	// Rate is the minimum gap between requests. Zero means no pacing.
	Rate    time.Duration
	Retries int

	last time.Time
}

// NewClient returns a Client with sensible defaults.
func NewClient() *Client {
	cfg := DefaultConfig()
	return &Client{
		HTTP:      &http.Client{Timeout: cfg.Timeout},
		UserAgent: cfg.UserAgent,
		BaseURL:   cfg.BaseURL,
		Rate:      cfg.Rate,
		Retries:   cfg.Retries,
	}
}

// Get fetches rawURL and returns the response body. It paces and retries
// according to the client's settings.
func (c *Client) Get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

// pace blocks until at least Rate has passed since the previous request.
func (c *Client) pace() {
	if c.Rate <= 0 {
		return
	}
	if wait := c.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// --- Output types ---

// KIndex is one planetary K-index reading.
type KIndex struct {
	Time         string  `json:"time" kit:"id"`
	Kp           float64 `json:"kp"`
	ARunning     float64 `json:"a_running"`
	StationCount int     `json:"station_count"`
	Activity     string  `json:"activity"`
}

// SolarWind is a solar wind speed reading.
type SolarWind struct {
	Time        string `json:"time" kit:"id"`
	ProtonSpeed int    `json:"proton_speed_km_s"`
}

// SpaceAlert is a NOAA space weather alert or watch.
type SpaceAlert struct {
	ProductID string `json:"product_id" kit:"id"`
	IssuedAt  string `json:"issued_at"`
	Message   string `json:"message"`
}

// SolarFlare is a record of a solar flare event.
type SolarFlare struct {
	BeginTime  string `json:"begin_time" kit:"id"`
	MaxTime    string `json:"max_time"`
	EndTime    string `json:"end_time"`
	BeginClass string `json:"begin_class"`
	MaxClass   string `json:"max_class"`
	Satellite  int    `json:"satellite"`
}

// kpActivity maps a Kp value to a human-readable activity description.
func kpActivity(kp float64) string {
	switch {
	case kp >= 9:
		return "Extreme Storm"
	case kp >= 8:
		return "Strong Storm"
	case kp >= 7:
		return "Moderate Storm"
	case kp >= 5:
		return "Minor Storm"
	case kp >= 4:
		return "Active"
	case kp >= 2:
		return "Unsettled"
	default:
		return "Quiet"
	}
}

// --- Wire types (JSON shapes from the API) ---

type wireKIndex struct {
	TimeTag      string  `json:"time_tag"`
	Kp           float64 `json:"Kp"`
	ARunning     float64 `json:"a_running"`
	StationCount int     `json:"station_count"`
}

type wireSolarWind struct {
	TimeTag     string `json:"time_tag"`
	ProtonSpeed int    `json:"proton_speed"`
}

type wireAlert struct {
	ProductID     string `json:"product_id"`
	IssueDateTime string `json:"issue_datetime"`
	Message       string `json:"message"`
}

type wireFlare struct {
	TimeTag    string `json:"time_tag"`
	BeginTime  string `json:"begin_time"`
	BeginClass string `json:"begin_class"`
	MaxTime    string `json:"max_time"`
	MaxClass   string `json:"max_class"`
	EndTime    string `json:"end_time"`
	EndClass   string `json:"end_class"`
	Satellite  int    `json:"satellite"`
}

// --- API methods ---

// GetKIndex fetches the planetary K-index history.
// It returns the last limit records (all if limit <= 0).
func (c *Client) GetKIndex(ctx context.Context, limit int) ([]KIndex, error) {
	url := c.BaseURL + "/products/noaa-planetary-k-index.json"
	body, err := c.Get(ctx, url)
	if err != nil {
		return nil, err
	}

	var wire []wireKIndex
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("decode k-index: %w", err)
	}

	out := make([]KIndex, 0, len(wire))
	for _, w := range wire {
		out = append(out, KIndex{
			Time:         w.TimeTag,
			Kp:           w.Kp,
			ARunning:     w.ARunning,
			StationCount: w.StationCount,
			Activity:     kpActivity(w.Kp),
		})
	}

	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}

// GetSolarWind fetches the current solar wind speed.
func (c *Client) GetSolarWind(ctx context.Context) (*SolarWind, error) {
	url := c.BaseURL + "/products/summary/solar-wind-speed.json"
	body, err := c.Get(ctx, url)
	if err != nil {
		return nil, err
	}

	var wire []wireSolarWind
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("decode solar-wind: %w", err)
	}
	if len(wire) == 0 {
		return nil, fmt.Errorf("no solar wind data returned")
	}

	w := wire[0]
	return &SolarWind{
		Time:        w.TimeTag,
		ProtonSpeed: w.ProtonSpeed,
	}, nil
}

// GetAlerts fetches current space weather alerts and watches.
// It returns up to limit records (all if limit <= 0).
func (c *Client) GetAlerts(ctx context.Context, limit int) ([]SpaceAlert, error) {
	url := c.BaseURL + "/products/alerts.json"
	body, err := c.Get(ctx, url)
	if err != nil {
		return nil, err
	}

	var wire []wireAlert
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("decode alerts: %w", err)
	}

	out := make([]SpaceAlert, 0, len(wire))
	for _, w := range wire {
		out = append(out, SpaceAlert{
			ProductID: w.ProductID,
			IssuedAt:  w.IssueDateTime,
			Message:   w.Message,
		})
	}

	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// GetFlares fetches solar flares from the last 7 days.
// Results are sorted descending by begin_time. It returns up to limit records (all if limit <= 0).
func (c *Client) GetFlares(ctx context.Context, limit int) ([]SolarFlare, error) {
	url := c.BaseURL + "/json/goes/primary/xray-flares-7-day.json"
	body, err := c.Get(ctx, url)
	if err != nil {
		return nil, err
	}

	var wire []wireFlare
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("decode flares: %w", err)
	}

	// Sort descending by begin_time (most recent first).
	sort.Slice(wire, func(i, j int) bool {
		return wire[i].BeginTime > wire[j].BeginTime
	})

	out := make([]SolarFlare, 0, len(wire))
	for _, w := range wire {
		out = append(out, SolarFlare{
			BeginTime:  w.BeginTime,
			MaxTime:    w.MaxTime,
			EndTime:    w.EndTime,
			BeginClass: w.BeginClass,
			MaxClass:   w.MaxClass,
			Satellite:  w.Satellite,
		})
	}

	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
