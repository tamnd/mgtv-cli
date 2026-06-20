// Package mgtv is the library behind the mgtv command: the HTTP client,
// pacing, and the typed data models for Mango TV (mgtv.com / 芒果TV).
//
// Mango TV is a major Chinese streaming platform. Content metadata is
// publicly accessible; no API key or account is required.
package mgtv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Host is the canonical hostname for the Mango TV site.
const Host = "mgtv.com"

// DefaultUserAgent mimics a real browser to avoid bot detection.
const DefaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

// ErrNotFound is returned when a clip ID does not exist.
var ErrNotFound = errors.New("not found")

// Video is one record from the Mango TV pcweb2 info API.
type Video struct {
	ClipID      string `json:"clip_id"`
	VideoID     string `json:"video_id"`
	Title       string `json:"title"`
	Type        string `json:"type"`
	Area        string `json:"area"`
	Genres      string `json:"genres"`
	Language    string `json:"language"`
	ReleaseDate string `json:"release_date"`
	UpdateInfo  string `json:"update_info"`
	Description string `json:"description"`
	CoverURL    string `json:"cover_url"`
	URL         string `json:"url"`
}

// Config holds constructor parameters for Client.
type Config struct {
	BaseURL   string
	APIURL    string
	SearchURL string
	UserAgent string
	Rate      time.Duration
	Retries   int
	Timeout   time.Duration
}

// DefaultConfig returns sensible defaults for Mango TV.
func DefaultConfig() Config {
	return Config{
		BaseURL:   "https://www.mgtv.com",
		APIURL:    "https://pcweb2.api.mgtv.com",
		SearchURL: "https://so.mgtv.com",
		UserAgent: DefaultUserAgent,
		Rate:      500 * time.Millisecond,
		Retries:   3,
		Timeout:   30 * time.Second,
	}
}

// Client is a rate-limited HTTP client for Mango TV.
type Client struct {
	cfg     Config
	http    *http.Client // follows redirects
	noRedir *http.Client // does NOT follow redirects
	mu      sync.Mutex
	last    time.Time
}

// NewClient returns a Client using DefaultConfig.
func NewClient() *Client {
	return NewClientConfig(DefaultConfig())
}

// NewClientConfig builds a Client from an explicit Config.
func NewClientConfig(cfg Config) *Client {
	transport := &http.Transport{}
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: cfg.Timeout, Transport: transport},
		noRedir: &http.Client{
			Timeout:   cfg.Timeout,
			Transport: transport,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// pace enforces the configured minimum gap between requests.
func (c *Client) pace() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cfg.Rate <= 0 {
		return
	}
	if wait := c.cfg.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

// get fetches a URL using the redirect-following client.
func (c *Client) get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			wait := time.Duration(attempt) * 500 * time.Millisecond
			if wait > 5*time.Second {
				wait = 5 * time.Second
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
		}
		body, retry, err := c.doGet(ctx, rawURL)
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

func (c *Client) doGet(ctx context.Context, rawURL string) ([]byte, bool, error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Referer", "https://www.mgtv.com/")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, false, ErrNotFound
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

// fetchVideoID fetches the videoId for a given clipId by reading the redirect
// Location header without following it.
func (c *Client) fetchVideoID(ctx context.Context, clipID string) (string, error) {
	c.pace()
	rawURL := c.cfg.BaseURL + "/b/" + clipID + ".html"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)

	resp, err := c.noRedir.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return "", ErrNotFound
	}

	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", nil
	}
	// Location: /b/<clipId>/<videoId>.html
	re := regexp.MustCompile(`/b/\d+/(\d+)\.html`)
	if m := re.FindStringSubmatch(loc); m != nil {
		return m[1], nil
	}
	return "", nil
}

// fetchVideoInfo calls the pcweb2 info API to get show metadata.
func (c *Client) fetchVideoInfo(ctx context.Context, clipID, videoID string) (*Video, error) {
	rawURL := fmt.Sprintf("%s/video/info?cid=%s&vid=%s&_support=10000000&platform=4&src=mgtv&version=5.5.35",
		c.cfg.APIURL, clipID, videoID)
	b, err := c.get(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	return parseVideoInfo(b, clipID, videoID)
}

// fetchCategoryClipIDs fetches a category page and extracts series clipIds.
func (c *Client) fetchCategoryClipIDs(ctx context.Context, rawURL string) ([]string, error) {
	b, err := c.get(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	return parseClipIDs(b), nil
}

// HotVideos returns trending videos from Mango TV category pages.
// category can be "variety", "tv", "movie", or "all".
func (c *Client) HotVideos(ctx context.Context, category string, limit int) ([]Video, error) {
	if limit <= 0 {
		limit = 20
	}

	var categoryURLs []string
	switch strings.ToLower(category) {
	case "variety", "show":
		categoryURLs = []string{c.cfg.BaseURL + "/show/"}
	case "tv", "drama":
		categoryURLs = []string{c.cfg.BaseURL + "/tv/"}
	case "movie":
		categoryURLs = []string{c.cfg.BaseURL + "/movie/"}
	default: // "all"
		categoryURLs = []string{
			c.cfg.BaseURL + "/show/",
			c.cfg.BaseURL + "/tv/",
			c.cfg.BaseURL + "/movie/",
		}
	}

	seen := map[string]bool{}
	var clipIDs []string
	for _, catURL := range categoryURLs {
		ids, err := c.fetchCategoryClipIDs(ctx, catURL)
		if err != nil {
			continue
		}
		for _, id := range ids {
			if !seen[id] {
				seen[id] = true
				clipIDs = append(clipIDs, id)
			}
		}
		if len(clipIDs) >= limit*2 {
			break
		}
	}

	return c.clipIDsToVideos(ctx, clipIDs, limit)
}

// SearchVideos searches Mango TV for content matching the query.
func (c *Client) SearchVideos(ctx context.Context, query string, limit int) ([]Video, error) {
	if limit <= 0 {
		limit = 20
	}
	rawURL := c.cfg.SearchURL + "/so/k-" + url.PathEscape(query)
	b, err := c.get(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	clipIDs := parseClipIDs(b)
	return c.clipIDsToVideos(ctx, clipIDs, limit)
}

// GetVideo fetches metadata for a single series by clip ID.
func (c *Client) GetVideo(ctx context.Context, clipID string) (*Video, error) {
	videoID, err := c.fetchVideoID(ctx, clipID)
	if err != nil {
		return nil, err
	}
	if videoID == "" {
		return nil, ErrNotFound
	}
	return c.fetchVideoInfo(ctx, clipID, videoID)
}

// clipIDsToVideos resolves up to limit clip IDs to Video records.
func (c *Client) clipIDsToVideos(ctx context.Context, clipIDs []string, limit int) ([]Video, error) {
	var videos []Video
	for _, clipID := range clipIDs {
		if ctx.Err() != nil {
			break
		}
		if len(videos) >= limit {
			break
		}
		v, err := c.GetVideo(ctx, clipID)
		if err != nil {
			continue // skip errors on individual clips
		}
		videos = append(videos, *v)
	}
	return videos, nil
}

// --- API response types ---

type apiInfoResponse struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data apiInfoData `json:"data"`
}

type apiInfoData struct {
	Info apiShowInfo `json:"info"`
}

type apiShowInfo struct {
	ClipID     string    `json:"clipId"`
	VideoID    string    `json:"videoId"`
	ClipName   string    `json:"clipName"`
	Title      string    `json:"title"`
	FstlvlType string    `json:"fstlvlType"`
	ClipImage  string    `json:"clipImage"`
	ClipImage2 string    `json:"clipImage2"`
	Detail     apiDetail `json:"detail"`
}

type apiDetail struct {
	Area        string `json:"area"`
	UpdateInfo  string `json:"updateInfo"`
	ReleaseTime string `json:"releaseTime"`
	Kind        string `json:"kind"`
	Language    string `json:"language"`
	Story       string `json:"story"`
	Director    string `json:"director"`
	Television  string `json:"television"`
	URL         string `json:"url"`
}

// parseVideoInfo decodes a pcweb2 API JSON response into a Video.
func parseVideoInfo(b []byte, clipID, videoID string) (*Video, error) {
	var resp apiInfoResponse
	if err := json.Unmarshal(b, &resp); err != nil {
		return nil, fmt.Errorf("decode video info: %w", err)
	}
	info := resp.Data.Info
	detail := info.Detail

	title := info.ClipName
	if title == "" {
		title = info.Title
	}

	cover := info.ClipImage
	if cover == "" {
		cover = info.ClipImage2
	}

	videoURL := ""
	if detail.URL != "" {
		videoURL = "https://www.mgtv.com" + detail.URL
	} else if clipID != "" && videoID != "" {
		videoURL = fmt.Sprintf("https://www.mgtv.com/b/%s/%s.html", clipID, videoID)
	}

	return &Video{
		ClipID:      info.ClipID,
		VideoID:     info.VideoID,
		Title:       title,
		Type:        info.FstlvlType,
		Area:        detail.Area,
		Genres:      detail.Kind,
		Language:    detail.Language,
		ReleaseDate: detail.ReleaseTime,
		UpdateInfo:  detail.UpdateInfo,
		Description: detail.Story,
		CoverURL:    cover,
		URL:         videoURL,
	}, nil
}
