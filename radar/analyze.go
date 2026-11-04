package radar

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"
)

// Status names used in reports.
const (
	StatusDry           = "dry"
	StatusPrecipitation = "precipitation"
	StatusNoCoverage    = "no_coverage"
)

// Intensity describes a reflectivity band.
type Intensity struct {
	MinDBZ int    `json:"min_dbz"`
	MaxDBZ int    `json:"max_dbz"`
	Label  string `json:"label" jsonschema:"very light, light, moderate, heavy or very heavy"`
	// Approximate rain rate range from the Marshall-Palmer relation Z=200R^1.6.
	MinMmH float64 `json:"min_mm_h"`
	MaxMmH float64 `json:"max_mm_h"`
}

func intensityOf(b Band) Intensity {
	return Intensity{
		MinDBZ: b.MinDBZ, MaxDBZ: b.MaxDBZ,
		Label:  label(b.MinDBZ),
		MinMmH: rainRate(b.MinDBZ),
		MaxMmH: rainRate(min(b.MaxDBZ, 75)),
	}
}

func label(dbz int) string {
	switch {
	case dbz < 18:
		return "very light"
	case dbz < 30:
		return "light"
	case dbz < 42:
		return "moderate"
	case dbz < 54:
		return "heavy"
	default:
		return "very heavy"
	}
}

// rainRate converts reflectivity to mm/h with Z=200R^1.6, rounded to 0.1.
func rainRate(dbz int) float64 {
	r := math.Pow(math.Pow(10, float64(dbz)/10)/200, 1/1.6)
	return math.Round(r*10) / 10
}

// Echo is precipitation found near the point of interest.
type Echo struct {
	Intensity
	DistanceKm float64 `json:"distance_km"`
	Direction  string  `json:"direction" jsonschema:"8-point compass direction from the point to the echo"`
	Lat        float64 `json:"lat"`
	Lon        float64 `json:"lon"`
}

// Snapshot is what one composite shows around a point.
type Snapshot struct {
	Time   time.Time `json:"time"`
	Status string    `json:"status" jsonschema:"dry, precipitation or no_coverage at the point itself"`
	// Here is the intensity at the point when Status is precipitation.
	Here *Intensity `json:"here,omitempty"`
	// Nearest is the closest precipitation within the radius, if any.
	Nearest *Echo `json:"nearest,omitempty"`
	// Strongest is the most intense precipitation within the radius, if any.
	Strongest *Echo `json:"strongest,omitempty"`
	// PrecipitationPercent is the share of the covered area within the
	// radius that shows precipitation.
	PrecipitationPercent float64 `json:"precipitation_percent"`
	// CoveragePercent is the share of the area within the radius that radar sees.
	CoveragePercent float64 `json:"coverage_percent"`
}

// Analyze describes what im shows at lat/lon and within radiusKm of it.
func (im *Image) Analyze(lat, lon, radiusKm float64) Snapshot {
	s := Snapshot{Time: im.Time, Status: StatusDry}
	switch st, b := im.at(lat, lon); st {
	case echo:
		s.Status = StatusPrecipitation
		in := intensityOf(b)
		s.Here = &in
	case noCoverage:
		s.Status = StatusNoCoverage
	}
	var a area
	im.scan(lat, lon, radiusKm, func(plat, plon, d float64, st pixelStatus, b Band) {
		a.add(lat, lon, plat, plon, d, st, b)
	})
	s.Nearest, s.Strongest = a.nearest, a.strongest
	s.CoveragePercent, s.PrecipitationPercent = a.percents()
	if s.Here != nil && s.Nearest != nil {
		s.Nearest.DistanceKm, s.Nearest.Direction = 0, "here"
	}
	return s
}

// area accumulates the pixels around a point.
type area struct {
	total, covered, wet int
	nearest, strongest  *Echo
}

func (a *area) add(lat, lon, plat, plon, d float64, st pixelStatus, b Band) {
	a.total++
	if st == noCoverage {
		return
	}
	a.covered++
	if st != echo {
		return
	}
	a.wet++
	e := Echo{Intensity: intensityOf(b), DistanceKm: round1(d), Direction: "here", Lat: round4(plat), Lon: round4(plon)}
	if d >= 0.5 {
		e.Direction = bearing(lat, lon, plat, plon)
	}
	if a.nearest == nil || e.DistanceKm < a.nearest.DistanceKm {
		a.nearest = &e
	}
	if a.strongest == nil || stronger(e, *a.strongest) {
		a.strongest = &e
	}
}

// stronger reports whether a is more intense than b, or as intense and closer.
func stronger(a, b Echo) bool {
	if a.MinDBZ != b.MinDBZ {
		return a.MinDBZ > b.MinDBZ
	}
	return a.DistanceKm < b.DistanceKm
}

func (a *area) percents() (coverage, precipitation float64) {
	if a.total > 0 {
		coverage = round1(100 * float64(a.covered) / float64(a.total))
	}
	if a.covered > 0 {
		precipitation = round1(100 * float64(a.wet) / float64(a.covered))
	}
	return coverage, precipitation
}

// scan calls fn for every pixel whose center is within radiusKm of lat/lon,
// with the pixel center, its distance and its classification.
func (im *Image) scan(lat, lon, radiusKm float64, fn func(plat, plon, d float64, st pixelStatus, b Band)) {
	dLat := radiusKm / 111.2
	dLon := radiusKm / (111.2 * math.Max(math.Cos(lat*math.Pi/180), 0.01))
	x0, y0 := im.clampPixel(lat+dLat, lon-dLon)
	x1, y1 := im.clampPixel(lat-dLat, lon+dLon)
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			plat, plon := im.center(x, y)
			d := distanceKm(lat, lon, plat, plon)
			if d > radiusKm {
				continue
			}
			st, b := im.classify(x, y)
			fn(plat, plon, d, st, b)
		}
	}
}

// clampPixel returns the pixel for lat/lon clamped to the image.
func (im *Image) clampPixel(lat, lon float64) (int, int) {
	b := im.bounds
	lat = math.Min(math.Max(lat, b.South+1e-9), b.North-1e-9)
	lon = math.Min(math.Max(lon, b.West+1e-9), b.East-1e-9)
	x, y, _ := im.pixel(lat, lon)
	return min(max(x, 0), im.w-1), min(max(y, 0), im.h-1)
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
func round4(v float64) float64 { return math.Round(v*1e4) / 1e4 }

// Trend summarises how precipitation near the point changed over the
// history window.
type Trend struct {
	// Movement is arrived, cleared or ongoing when the point itself got,
	// lost or kept precipitation; otherwise approaching, receding or steady
	// from the distance to the nearest precipitation, appeared or dissipated
	// when it entered or left the radius, none when there was none, and
	// unknown without history.
	Movement string `json:"movement" jsonschema:"arrived, cleared, ongoing, approaching, receding, steady, appeared, dissipated, none or unknown"`
	// NearestChangeKm is how much the nearest precipitation moved closer
	// (negative) or farther (positive) over the window.
	NearestChangeKm *float64 `json:"nearest_change_km,omitempty"`
	// SpeedKmh is the approach or retreat speed when it can be estimated.
	SpeedKmh *float64 `json:"speed_kmh,omitempty"`
	// EtaMinutes estimates when approaching precipitation reaches the point.
	EtaMinutes *int `json:"eta_minutes,omitempty"`
	// AreaChangePercent is the change in PrecipitationPercent over the window.
	AreaChangePercent float64 `json:"area_change_percent"`
	// WindowMinutes is the time between the oldest and newest snapshot.
	WindowMinutes int `json:"window_minutes"`
}

// steadyKm is the distance change, about two pixels, below which
// precipitation is considered steady.
const steadyKm = 4

// TrendOf compares the oldest and newest of snapshots, oldest first.
func TrendOf(snaps []Snapshot) Trend {
	if len(snaps) < 2 {
		return Trend{Movement: "unknown"}
	}
	first, last := snaps[0], snaps[len(snaps)-1]
	window := last.Time.Sub(first.Time)
	t := Trend{
		WindowMinutes:     int(window.Minutes()),
		AreaChangePercent: round1(last.PrecipitationPercent - first.PrecipitationPercent),
	}
	if first.Nearest != nil && last.Nearest != nil {
		change := round1(last.Nearest.DistanceKm - first.Nearest.DistanceKm)
		t.NearestChangeKm = &change
	}
	t.Movement = movement(first, last, t.NearestChangeKm)
	if (t.Movement == "approaching" || t.Movement == "receding") && window > 0 {
		speed := round1(math.Abs(*t.NearestChangeKm) / window.Hours())
		t.SpeedKmh = &speed
		if t.Movement == "approaching" && speed > 0 {
			eta := int(math.Round(last.Nearest.DistanceKm / speed * 60))
			t.EtaMinutes = &eta
		}
	}
	return t
}

func movement(first, last Snapshot, change *float64) string {
	switch {
	case first.Here == nil && last.Here != nil:
		return "arrived"
	case first.Here != nil && last.Here == nil:
		return "cleared"
	case first.Here != nil:
		return "ongoing"
	case first.Nearest == nil && last.Nearest == nil:
		return "none"
	case first.Nearest == nil:
		return "appeared"
	case last.Nearest == nil:
		return "dissipated"
	case *change <= -steadyKm:
		return "approaching"
	case *change >= steadyKm:
		return "receding"
	}
	return "steady"
}

// Report is the radar picture around a point.
type Report struct {
	// AgeMinutes is how old the latest composite is.
	AgeMinutes int      `json:"age_minutes"`
	Latest     Snapshot `json:"latest"`
	// History holds earlier snapshots, oldest first, ending with Latest.
	History []Snapshot `json:"history,omitempty"`
	Trend   Trend      `json:"trend"`
}

// Options controls Check.
type Options struct {
	// RadiusKm is the search radius around the point; default DefaultRadiusKm.
	RadiusKm float64
	// HistoryMinutes is how far back to look for the trend; default
	// DefaultHistoryMinutes. Set NoHistory to use only the latest composite.
	HistoryMinutes int
	NoHistory      bool
}

// DefaultRadiusKm is the search radius when Options.RadiusKm is zero.
const DefaultRadiusKm = 25

// MaxRadiusKm bounds Options.RadiusKm.
const MaxRadiusKm = 200

// DefaultHistoryMinutes is the trend window when Options.HistoryMinutes is zero.
const DefaultHistoryMinutes = 60

// MaxHistoryMinutes bounds Options.HistoryMinutes.
const MaxHistoryMinutes = 180

// Check fetches the latest composites and reports precipitation at and
// around lat/lon.
func (c *Client) Check(ctx context.Context, lat, lon float64, opt Options) (Report, error) {
	if err := validate(lat, lon, &opt); err != nil {
		return Report{}, err
	}
	frames, err := c.Frames(ctx)
	if err != nil {
		return Report{}, err
	}
	frames = window(frames, opt)
	images, err := c.images(ctx, frames)
	if err != nil {
		return Report{}, err
	}
	snaps := make([]Snapshot, 0, len(images))
	for _, im := range images {
		snaps = append(snaps, im.Analyze(lat, lon, opt.RadiusKm))
	}
	latest := snaps[len(snaps)-1]
	r := Report{
		AgeMinutes: max(0, int(c.clock().Sub(latest.Time).Minutes())),
		Latest:     latest,
		Trend:      TrendOf(snaps),
	}
	if len(snaps) > 1 {
		r.History = snaps
	}
	return r, nil
}

func validate(lat, lon float64, opt *Options) error {
	if !validCoord(lat, 90) || !validCoord(lon, 180) {
		return fmt.Errorf("invalid coordinates %v, %v", lat, lon)
	}
	if opt.RadiusKm == 0 {
		opt.RadiusKm = DefaultRadiusKm
	}
	if !validCoord(opt.RadiusKm, MaxRadiusKm) || opt.RadiusKm < 0 {
		return fmt.Errorf("radius must be between 0 and %d km", MaxRadiusKm)
	}
	if opt.HistoryMinutes == 0 && !opt.NoHistory {
		opt.HistoryMinutes = DefaultHistoryMinutes
	}
	if opt.HistoryMinutes < 0 || opt.HistoryMinutes > MaxHistoryMinutes {
		return fmt.Errorf("history must be between 0 and %d minutes", MaxHistoryMinutes)
	}
	return nil
}

// validCoord reports whether v is a number within [-limit, limit].
func validCoord(v, limit float64) bool {
	return !math.IsNaN(v) && v >= -limit && v <= limit
}

// window returns the frames within opt.HistoryMinutes of the newest one.
func window(frames []Frame, opt Options) []Frame {
	last := frames[len(frames)-1]
	from := last.Time.Add(-time.Duration(opt.HistoryMinutes) * time.Minute)
	i := len(frames) - 1
	for i > 0 && !frames[i-1].Time.Before(from) {
		i--
	}
	return frames[i:]
}

// images fetches frames concurrently. Older frames that fail are skipped;
// the newest must succeed.
func (c *Client) images(ctx context.Context, frames []Frame) ([]*Image, error) {
	// Fetch shared metadata once before going concurrent.
	if _, err := c.Legend(ctx); err != nil {
		return nil, err
	}
	if _, err := c.boundsFor(ctx, frames[len(frames)-1]); err != nil {
		return nil, err
	}
	out := make([]*Image, len(frames))
	errs := make([]error, len(frames))
	var wg sync.WaitGroup
	for i, f := range frames {
		wg.Go(func() { out[i], errs[i] = c.Image(ctx, f) })
	}
	wg.Wait()
	if err := errs[len(errs)-1]; err != nil {
		return nil, err
	}
	var ok []*Image
	for i, im := range out {
		if errs[i] == nil {
			ok = append(ok, im)
		}
	}
	if len(ok) == 0 {
		return nil, errors.New("radar: no images")
	}
	return ok, nil
}
