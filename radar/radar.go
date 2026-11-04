// Package radar reads AEMET's national weather radar composite for
// mainland Spain and the Balearic Islands and answers questions about a
// point: is it raining there, how hard, where is the nearest rain and is it
// getting closer.
//
// It uses the public endpoints behind https://www.aemet.es/es/eltiempo/observacion/radar,
// which need no API key. A new composite is published every 10 minutes.
// The Canary Islands are not part of the composite.
package radar

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

// DefaultBaseURL is the root of AEMET's web radar API.
const DefaultBaseURL = "https://www.aemet.es/es/api-eltiempo/radar"

const (
	product = "compo" // national reflectivity composite
	region  = "PB"    // Península y Baleares

	maxJSONBytes  = 1 << 20
	maxImageBytes = 4 << 20
	maxImageSide  = 4096
)

// Frame is one published radar composite.
type Frame struct {
	Time time.Time
	File string
}

// Band is a reflectivity interval of the radar color scale.
type Band struct {
	MinDBZ int
	MaxDBZ int
	Color  color.RGBA
}

// Client fetches radar data from AEMET. The zero value is ready to use.
type Client struct {
	// HTTPClient is used for requests; nil means a client with a 30s timeout.
	HTTPClient *http.Client
	// BaseURL overrides DefaultBaseURL, for tests.
	BaseURL string

	now func() time.Time // for tests

	mu     sync.Mutex
	legend []Band
	bounds *Bounds
}

func (c *Client) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

var defaultHTTPClient = &http.Client{Timeout: 30 * time.Second}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return defaultHTTPClient
}

func (c *Client) url(path string) string {
	base := c.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	return strings.TrimSuffix(base, "/") + "/" + path
}

// get fetches path and returns at most limit bytes of its body.
func (c *Client) get(ctx context.Context, path string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url(path), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("radar: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("radar: GET %s: HTTP %d", path, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("radar: GET %s: %w", path, err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("radar: GET %s: response larger than %d bytes", path, limit)
	}
	return body, nil
}

func (c *Client) getJSON(ctx context.Context, path string, v any) error {
	body, err := c.get(ctx, path, maxJSONBytes)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("radar: decode %s: %w", path, err)
	}
	return nil
}

// Frames returns the published composites of roughly the last day, oldest first.
func (c *Client) Frames(ctx context.Context) ([]Frame, error) {
	var resp []struct {
		Elementos []struct {
			Fecha   string `json:"Fecha"`
			Fichero string `json:"Nombre fichero"`
		} `json:"Elementos"`
	}
	if err := c.getJSON(ctx, "timeline/"+product+"/"+region, &resp); err != nil {
		return nil, err
	}
	if len(resp) == 0 {
		return nil, errors.New("radar: empty timeline")
	}
	frames := make([]Frame, 0, len(resp[0].Elementos))
	for _, e := range resp[0].Elementos {
		t, err := time.Parse(time.RFC3339, e.Fecha)
		if err != nil || !validFile(e.Fichero) {
			continue
		}
		frames = append(frames, Frame{Time: t, File: e.Fichero})
	}
	if len(frames) == 0 {
		return nil, errors.New("radar: no usable frames in timeline")
	}
	slices.SortFunc(frames, func(a, b Frame) int { return a.Time.Compare(b.Time) })
	return frames, nil
}

// validFile accepts the plain file names AEMET publishes, so a name from the
// timeline cannot change the request path.
func validFile(name string) bool {
	return fileRe.MatchString(name) && !strings.Contains(name, "..")
}

var fileRe = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

// Legend returns the reflectivity color scale, weakest first. It is cached.
func (c *Client) Legend(ctx context.Context) ([]Band, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.legend != nil {
		return c.legend, nil
	}
	var resp struct {
		Lista []struct {
			Valores []float64 `json:"Valores"`
			RGBA    []string  `json:"RGBA"`
		} `json:"Lista RGBA"`
	}
	if err := c.getJSON(ctx, "leyenda-radar/"+product, &resp); err != nil {
		return nil, err
	}
	var bands []Band
	for _, e := range resp.Lista {
		b, err := parseBand(e.Valores, e.RGBA)
		if err != nil {
			return nil, err
		}
		bands = append(bands, b)
	}
	if len(bands) == 0 {
		return nil, errors.New("radar: empty legend")
	}
	slices.SortFunc(bands, func(a, b Band) int { return a.MinDBZ - b.MinDBZ })
	c.legend = bands
	return bands, nil
}

func parseBand(vals []float64, rgba []string) (Band, error) {
	if len(vals) != 2 || len(rgba) < 3 {
		return Band{}, fmt.Errorf("radar: malformed legend entry %v %v", vals, rgba)
	}
	var ch [3]uint8
	for i := range ch {
		var v int
		if _, err := fmt.Sscan(rgba[i], &v); err != nil || v < 0 || v > 255 {
			return Band{}, fmt.Errorf("radar: malformed legend color %v", rgba)
		}
		ch[i] = uint8(v)
	}
	return Band{
		MinDBZ: int(vals[0]),
		MaxDBZ: int(vals[1]),
		Color:  color.RGBA{R: ch[0], G: ch[1], B: ch[2], A: 255},
	}, nil
}

// Bounds is the geographic extent of a composite image in degrees.
type Bounds struct {
	West, South, East, North float64
}

// boundsFor returns the extent of frame f. AEMET returns the same extent for
// every composite, so the first answer is cached.
func (c *Client) boundsFor(ctx context.Context, f Frame) (Bounds, error) {
	c.mu.Lock()
	if c.bounds != nil {
		b := *c.bounds
		c.mu.Unlock()
		return b, nil
	}
	c.mu.Unlock()

	var corners [][]float64 // [lon, lat] pairs
	if err := c.getJSON(ctx, "bounds-radar/"+product+"/"+f.File, &corners); err != nil {
		return Bounds{}, err
	}
	b, err := cornersToBounds(corners)
	if err != nil {
		return Bounds{}, err
	}
	c.mu.Lock()
	c.bounds = &b
	c.mu.Unlock()
	return b, nil
}

func cornersToBounds(corners [][]float64) (Bounds, error) {
	if len(corners) < 2 {
		return Bounds{}, errors.New("radar: malformed image bounds")
	}
	b := Bounds{West: 180, South: 90, East: -180, North: -90}
	for _, p := range corners {
		if len(p) != 2 {
			return Bounds{}, errors.New("radar: malformed image bounds")
		}
		b.West, b.East = min(b.West, p[0]), max(b.East, p[0])
		b.South, b.North = min(b.South, p[1]), max(b.North, p[1])
	}
	if b.West >= b.East || b.South >= b.North || b.South <= -85 || b.North >= 85 {
		return Bounds{}, fmt.Errorf("radar: invalid image bounds %+v", b)
	}
	return b, nil
}

// Image fetches and decodes the composite for frame f.
func (c *Client) Image(ctx context.Context, f Frame) (*Image, error) {
	if !validFile(f.File) {
		return nil, fmt.Errorf("radar: invalid frame file %q", f.File)
	}
	legend, err := c.Legend(ctx)
	if err != nil {
		return nil, err
	}
	bounds, err := c.boundsFor(ctx, f)
	if err != nil {
		return nil, err
	}
	body, err := c.get(ctx, "imagen-radar/"+product+"/"+f.File, maxImageBytes)
	if err != nil {
		return nil, err
	}
	img, err := decodePNG(body)
	if err != nil {
		return nil, fmt.Errorf("radar: %s: %w", f.File, err)
	}
	return newImage(f.Time, img, bounds, legend), nil
}

func decodePNG(body []byte) (image.Image, error) {
	cfg, err := png.DecodeConfig(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxImageSide || cfg.Height > maxImageSide {
		return nil, fmt.Errorf("unexpected image size %dx%d", cfg.Width, cfg.Height)
	}
	return png.Decode(bytes.NewReader(body))
}
