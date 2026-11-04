package radar

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rubiojr/aemet-go/internal/radartest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Coordinates used against the fixture composites of 2026-11-03 22:40-23:50 UTC.
const (
	masnouLat, masnouLon = 41.4794, 2.3165
	madridLat, madridLon = 40.4168, -3.7038
	canaryLat, canaryLon = 28.1, -15.43
)

type fixture struct {
	*radartest.Server
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	return fixture{radartest.NewServer(t)}
}

// fixedNow is 8 minutes after the newest fixture composite.
var fixedNow = radartest.Latest.Add(8 * time.Minute)

func (f fixture) client() *Client {
	return &Client{BaseURL: f.URL, HTTPClient: f.Server.Client(), now: func() time.Time { return fixedNow }}
}

func TestFrames(t *testing.T) {
	c := newFixture(t).client()
	frames, err := c.Frames(context.Background())
	require.NoError(t, err)
	// Entries with a bad date or an unsafe file name are dropped.
	require.Len(t, frames, 8)
	assert.Equal(t, "radw202611032240_3857.png", frames[0].File)
	assert.Equal(t, "radw202611032350_3857.png", frames[7].File)
	assert.True(t, frames[7].Time.Equal(radartest.Latest))
	for i := 1; i < len(frames); i++ {
		assert.True(t, frames[i-1].Time.Before(frames[i].Time), "frames sorted oldest first")
	}
}

func TestLegend(t *testing.T) {
	f := newFixture(t)
	c := f.client()
	bands, err := c.Legend(context.Background())
	require.NoError(t, err)
	require.Len(t, bands, 11)
	assert.Equal(t, Band{MinDBZ: 12, MaxDBZ: 18, Color: color.RGBA{0, 0, 252, 255}}, bands[0])
	assert.Equal(t, Band{MinDBZ: 72, MaxDBZ: 150, Color: color.RGBA{200, 0, 90, 255}}, bands[10])

	n := f.Requests()
	_, err = c.Legend(context.Background())
	require.NoError(t, err)
	assert.Equal(t, n, f.Requests(), "legend is cached")
}

func TestCheckRaining(t *testing.T) {
	c := newFixture(t).client()
	r, err := c.Check(context.Background(), masnouLat, masnouLon, Options{})
	require.NoError(t, err)

	assert.Equal(t, 8, r.AgeMinutes)
	l := r.Latest
	assert.True(t, l.Time.Equal(radartest.Latest))
	assert.Equal(t, StatusPrecipitation, l.Status)
	require.NotNil(t, l.Here)
	assert.Equal(t, Intensity{MinDBZ: 30, MaxDBZ: 36, Label: "moderate", MinMmH: 2.7, MaxMmH: 6.5}, *l.Here)
	require.NotNil(t, l.Nearest)
	assert.Equal(t, "here", l.Nearest.Direction)
	assert.Zero(t, l.Nearest.DistanceKm)
	require.NotNil(t, l.Strongest)
	assert.Equal(t, "heavy", l.Strongest.Label)
	assert.Equal(t, 42, l.Strongest.MinDBZ)
	assert.Equal(t, "E", l.Strongest.Direction)
	assert.InDelta(t, 5.9, l.Strongest.DistanceKm, 0.5)
	assert.Equal(t, 100.0, l.CoveragePercent)
	assert.InDelta(t, 83.5, l.PrecipitationPercent, 1)

	// The default window is one hour: seven composites, 22:50 to 23:50.
	require.Len(t, r.History, 7)
	assert.Equal(t, l, r.History[6])
	assert.Equal(t, 60, r.Trend.WindowMinutes)
	assert.Equal(t, "ongoing", r.Trend.Movement)
}

func TestCheckDryArea(t *testing.T) {
	c := newFixture(t).client()
	r, err := c.Check(context.Background(), madridLat, madridLon, Options{})
	require.NoError(t, err)
	assert.Equal(t, StatusDry, r.Latest.Status)
	assert.Nil(t, r.Latest.Here)
	assert.Nil(t, r.Latest.Nearest)
	assert.Equal(t, 100.0, r.Latest.CoveragePercent)
	assert.Equal(t, "none", r.Trend.Movement)
	// 23:10 paints covered dry areas translucent black; that is dry too.
	for _, s := range r.History {
		assert.Equal(t, StatusDry, s.Status, s.Time)
	}
}

func TestCheckOutsideCoverage(t *testing.T) {
	c := newFixture(t).client()
	r, err := c.Check(context.Background(), canaryLat, canaryLon, Options{NoHistory: true})
	require.NoError(t, err)
	assert.Equal(t, StatusNoCoverage, r.Latest.Status)
	assert.Zero(t, r.Latest.CoveragePercent)
	assert.Empty(t, r.History)
	assert.Equal(t, "unknown", r.Trend.Movement)

	// Points outside the image have no coverage instead of failing.
	r, err = c.Check(context.Background(), 60, 30, Options{NoHistory: true})
	require.NoError(t, err)
	assert.Equal(t, StatusNoCoverage, r.Latest.Status)
}

func TestCheckArrived(t *testing.T) {
	c := newFixture(t).client()
	// Dry at 22:50, rain from 23:40.
	r, err := c.Check(context.Background(), 41.70, 2.50, Options{})
	require.NoError(t, err)
	assert.Equal(t, StatusDry, r.History[0].Status)
	assert.Equal(t, StatusPrecipitation, r.Latest.Status)
	assert.Equal(t, "arrived", r.Trend.Movement)
	assert.Greater(t, r.Trend.AreaChangePercent, 0.0)
}

func TestCheckSkipsFailedOlderFrames(t *testing.T) {
	f := newFixture(t)
	f.Fail("radw202611032300_3857.png")
	r, err := f.client().Check(context.Background(), masnouLat, masnouLon, Options{})
	require.NoError(t, err)
	assert.Len(t, r.History, 6)

	f.Fail("radw202611032350_3857.png")
	_, err = f.client().Check(context.Background(), masnouLat, masnouLon, Options{})
	assert.ErrorContains(t, err, "HTTP 500")
}

func TestCheckValidation(t *testing.T) {
	c := newFixture(t).client()
	ctx := context.Background()
	tests := []struct {
		name     string
		lat, lon float64
		opt      Options
		want     string
	}{
		{"latitude", 91, 0, Options{}, "invalid coordinates"},
		{"longitude", 0, -181, Options{}, "invalid coordinates"},
		{"negative radius", 40, -3, Options{RadiusKm: -1}, "radius"},
		{"huge radius", 40, -3, Options{RadiusKm: MaxRadiusKm + 1}, "radius"},
		{"history", 40, -3, Options{HistoryMinutes: MaxHistoryMinutes + 1}, "history"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := c.Check(ctx, tt.lat, tt.lon, tt.opt)
			assert.ErrorContains(t, err, tt.want)
		})
	}
}

func TestHTTPErrors(t *testing.T) {
	ctx := context.Background()
	serve := func(t *testing.T, body string, status int) *Client {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, body)
		}))
		t.Cleanup(srv.Close)
		return &Client{BaseURL: srv.URL}
	}
	t.Run("status", func(t *testing.T) {
		_, err := serve(t, "", http.StatusBadGateway).Frames(ctx)
		assert.ErrorContains(t, err, "HTTP 502")
	})
	t.Run("bad json", func(t *testing.T) {
		_, err := serve(t, "<html>", http.StatusOK).Frames(ctx)
		assert.ErrorContains(t, err, "decode")
	})
	t.Run("empty timeline", func(t *testing.T) {
		_, err := serve(t, "[]", http.StatusOK).Frames(ctx)
		assert.ErrorContains(t, err, "empty timeline")
	})
	t.Run("no usable frames", func(t *testing.T) {
		_, err := serve(t, `[{"Elementos":[{"Fecha":"x","Nombre fichero":"a.png"}]}]`, http.StatusOK).Frames(ctx)
		assert.ErrorContains(t, err, "no usable frames")
	})
	t.Run("oversized", func(t *testing.T) {
		_, err := serve(t, strings.Repeat(" ", maxJSONBytes+1), http.StatusOK).Frames(ctx)
		assert.ErrorContains(t, err, "larger than")
	})
	t.Run("empty legend", func(t *testing.T) {
		_, err := serve(t, `{"Lista RGBA":[]}`, http.StatusOK).Legend(ctx)
		assert.ErrorContains(t, err, "empty legend")
	})
	t.Run("bad legend color", func(t *testing.T) {
		_, err := serve(t, `{"Lista RGBA":[{"Valores":[12,18],"RGBA":["0","x","252","255"]}]}`, http.StatusOK).Legend(ctx)
		assert.ErrorContains(t, err, "legend color")
	})
	t.Run("bad legend entry", func(t *testing.T) {
		_, err := serve(t, `{"Lista RGBA":[{"Valores":[12],"RGBA":["0","0","252"]}]}`, http.StatusOK).Legend(ctx)
		assert.ErrorContains(t, err, "legend entry")
	})
}

func TestImageRejectsBadInput(t *testing.T) {
	c := newFixture(t).client()
	_, err := c.Image(context.Background(), Frame{File: "../legend.json"})
	assert.ErrorContains(t, err, "invalid frame file")

	_, err = decodePNG([]byte("not a png"))
	assert.Error(t, err)

	big := image.NewPaletted(image.Rect(0, 0, maxImageSide+1, 1), color.Palette{color.Black})
	pr, pw := io.Pipe()
	go func() { _ = pw.CloseWithError(png.Encode(pw, big)) }()
	body, err := io.ReadAll(pr)
	require.NoError(t, err)
	_, err = decodePNG(body)
	assert.ErrorContains(t, err, "unexpected image size")
}

func TestCornersToBounds(t *testing.T) {
	b, err := cornersToBounds([][]float64{{12.14, 27.22}, {12.14, 51.3}, {-16.08, 51.3}, {-16.08, 27.22}})
	require.NoError(t, err)
	assert.Equal(t, Bounds{West: -16.08, South: 27.22, East: 12.14, North: 51.3}, b)

	for _, bad := range [][][]float64{nil, {{1, 2}}, {{1, 2, 3}, {4, 5}}, {{1, 2}, {1, 2}}, {{0, -89}, {1, 89}}} {
		_, err := cornersToBounds(bad)
		assert.Error(t, err, "%v", bad)
	}
}

func TestPixelMatchesGeoTIFF(t *testing.T) {
	// AEMET also publishes 22:40 as an EPSG:4326 GeoTIFF (origin -12.11, 46.30,
	// 0.026331764° pixels). In it El Masnou is dry with 18-24 dBZ one pixel
	// west; the Mercator PNG must agree.
	c := newFixture(t).client()
	im, err := c.Image(context.Background(), Frame{File: "radw202611032240_3857.png"})
	require.NoError(t, err)
	st, _ := im.at(masnouLat, masnouLon)
	assert.Equal(t, dry, st)
	s := im.Analyze(masnouLat, masnouLon, 5)
	require.NotNil(t, s.Nearest)
	assert.Equal(t, 18, s.Nearest.MinDBZ)
	assert.Contains(t, []string{"W", "SW"}, s.Nearest.Direction)
	assert.InDelta(t, 2, s.Nearest.DistanceKm, 1)
}

func TestTrendOf(t *testing.T) {
	t0 := time.Date(2026, 11, 3, 22, 0, 0, 0, time.UTC)
	echoAt := func(km float64) *Echo { return &Echo{DistanceKm: km} }
	here := &Intensity{}
	snap := func(min int, h *Intensity, n *Echo, pct float64) Snapshot {
		return Snapshot{Time: t0.Add(time.Duration(min) * time.Minute), Here: h, Nearest: n, PrecipitationPercent: pct}
	}
	tests := []struct {
		name  string
		snaps []Snapshot
		want  string
	}{
		{"single", []Snapshot{snap(0, nil, nil, 0)}, "unknown"},
		{"none", []Snapshot{snap(0, nil, nil, 0), snap(60, nil, nil, 0)}, "none"},
		{"arrived", []Snapshot{snap(0, nil, echoAt(10), 5), snap(60, here, echoAt(0), 20)}, "arrived"},
		{"cleared", []Snapshot{snap(0, here, echoAt(0), 20), snap(60, nil, echoAt(8), 5)}, "cleared"},
		{"ongoing", []Snapshot{snap(0, here, echoAt(0), 20), snap(60, here, echoAt(0), 20)}, "ongoing"},
		{"appeared", []Snapshot{snap(0, nil, nil, 0), snap(60, nil, echoAt(12), 3)}, "appeared"},
		{"dissipated", []Snapshot{snap(0, nil, echoAt(12), 3), snap(60, nil, nil, 0)}, "dissipated"},
		{"steady", []Snapshot{snap(0, nil, echoAt(12), 3), snap(60, nil, echoAt(10), 3)}, "steady"},
		{"receding", []Snapshot{snap(0, nil, echoAt(5), 3), snap(60, nil, echoAt(20), 3)}, "receding"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, TrendOf(tt.snaps).Movement)
		})
	}

	tr := TrendOf([]Snapshot{snap(0, nil, echoAt(30), 2), snap(30, nil, echoAt(20), 6)})
	assert.Equal(t, "approaching", tr.Movement)
	assert.Equal(t, 30, tr.WindowMinutes)
	require.NotNil(t, tr.NearestChangeKm)
	assert.Equal(t, -10.0, *tr.NearestChangeKm)
	require.NotNil(t, tr.SpeedKmh)
	assert.Equal(t, 20.0, *tr.SpeedKmh)
	require.NotNil(t, tr.EtaMinutes)
	assert.Equal(t, 60, *tr.EtaMinutes)
	assert.Equal(t, 4.0, tr.AreaChangePercent)

	tr = TrendOf([]Snapshot{snap(0, nil, echoAt(5), 3), snap(60, nil, echoAt(20), 3)})
	require.NotNil(t, tr.SpeedKmh)
	assert.Equal(t, 15.0, *tr.SpeedKmh)
	assert.Nil(t, tr.EtaMinutes)
}

func TestIntensity(t *testing.T) {
	assert.Equal(t, "very light", label(12))
	assert.Equal(t, "light", label(24))
	assert.Equal(t, "moderate", label(36))
	assert.Equal(t, "heavy", label(48))
	assert.Equal(t, "very heavy", label(60))
	assert.Equal(t, 0.2, rainRate(12))
	assert.Equal(t, 2.7, rainRate(30))
	// The open-ended top band is capped so the rate stays finite and sane.
	assert.Equal(t, rainRate(75), intensityOf(Band{MinDBZ: 72, MaxDBZ: 150}).MaxMmH)
}

func TestGeo(t *testing.T) {
	// Madrid to Barcelona is about 505 km away, bearing about 74°.
	assert.InDelta(t, 505, distanceKm(40.4168, -3.7038, 41.3874, 2.1686), 5)
	assert.Equal(t, "E", bearing(40.4168, -3.7038, 41.3874, 2.1686))
	assert.Equal(t, "NE", bearing(40, 0, 41, 1.3))
	assert.Equal(t, "N", bearing(40, 0, 41, 0))
	assert.Equal(t, "S", bearing(41, 0, 40, 0))
	assert.Equal(t, "W", bearing(40, 0, 40, -1))
	assert.InDelta(t, 40.0, invMercY(mercY(40)), 1e-9)
}

func TestValidFile(t *testing.T) {
	assert.True(t, validFile("radw202611032350_3857.png"))
	for _, bad := range []string{"", "../x.png", "a/b.png", "a..png", "a?b", strings.Repeat("a", 65)} {
		assert.False(t, validFile(bad), bad)
	}
}

func TestDefaultClient(t *testing.T) {
	var c Client
	assert.Equal(t, defaultHTTPClient, c.httpClient())
	assert.Equal(t, DefaultBaseURL+"/x", c.url("x"))
	assert.WithinDuration(t, time.Now(), c.clock(), time.Minute)
}
