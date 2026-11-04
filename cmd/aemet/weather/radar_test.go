package weather

import (
	"testing"
	"time"

	"github.com/rubiojr/aemet-go/radar"
	"github.com/stretchr/testify/assert"
)

func TestRadarSummary(t *testing.T) {
	at := time.Date(2026, 11, 3, 23, 50, 0, 0, time.UTC)
	light := radar.Intensity{MinDBZ: 18, MaxDBZ: 24, Label: "light", MinMmH: 0.5, MaxMmH: 1.2}
	heavy := radar.Intensity{MinDBZ: 42, MaxDBZ: 48, Label: "heavy", MinMmH: 15.4, MaxMmH: 36.5}
	km := func(v float64) *float64 { return &v }
	eta := 45

	tests := []struct {
		name string
		r    radar.Report
		want string
	}{
		{
			"no coverage",
			radar.Report{AgeMinutes: 8, Latest: radar.Snapshot{Time: at, Status: radar.StatusNoCoverage}},
			"Radar 00:50 CET (8 min old): no radar coverage at X.",
		},
		{
			"dry, nothing near",
			radar.Report{Latest: radar.Snapshot{Time: at, Status: radar.StatusDry}, Trend: radar.Trend{Movement: "none", WindowMinutes: 60}},
			"Radar 00:50 CET (0 min old): dry at X, no precipitation within 25 km.",
		},
		{
			"dry, rain approaching",
			radar.Report{
				Latest: radar.Snapshot{
					Time: at, Status: radar.StatusDry,
					Nearest:   &radar.Echo{Intensity: light, DistanceKm: 12.5, Direction: "W"},
					Strongest: &radar.Echo{Intensity: heavy, DistanceKm: 20, Direction: "SW"},
				},
				Trend: radar.Trend{Movement: "approaching", NearestChangeKm: km(-10), EtaMinutes: &eta, WindowMinutes: 60},
			},
			"Radar 00:50 CET (0 min old): dry at X; nearest precipitation light, 12.5 km W. " +
				"Strongest within 25 km: heavy (42-48 dBZ, ~15.4-36.5 mm/h), 20 km SW. " +
				"Approaching: 10 km closer in 60 min, could arrive in about 45 min.",
		},
		{
			"raining, heavier nearby",
			radar.Report{
				Latest: radar.Snapshot{
					Time: at, Status: radar.StatusPrecipitation, Here: &light,
					Nearest:   &radar.Echo{Intensity: light, Direction: "here"},
					Strongest: &radar.Echo{Intensity: heavy, DistanceKm: 5.9, Direction: "E"},
				},
				Trend: radar.Trend{Movement: "arrived", WindowMinutes: 60},
			},
			"Radar 00:50 CET (0 min old): Light precipitation at X (18-24 dBZ, ~0.5-1.2 mm/h). " +
				"Strongest within 25 km: heavy (42-48 dBZ, ~15.4-36.5 mm/h), 5.9 km E. Started within the last 60 min.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, RadarSummary("X", 25, tt.r))
		})
	}
}

func TestTrendText(t *testing.T) {
	km := func(v float64) *float64 { return &v }
	tests := map[string]radar.Trend{
		"Moving away: 15 km farther in 60 min.":                        {Movement: "receding", NearestChangeKm: km(15), WindowMinutes: 60},
		"Nearest precipitation about the same distance as 60 min ago.": {Movement: "steady", WindowMinutes: 60},
		"Stopped within the last 60 min.":                              {Movement: "cleared", WindowMinutes: 60},
		"Raining throughout the last 60 min.":                          {Movement: "ongoing", WindowMinutes: 60},
		"Precipitation appeared nearby within the last 60 min.":        {Movement: "appeared", WindowMinutes: 60},
		"Nearby precipitation dissipated within the last 60 min.":      {Movement: "dissipated", WindowMinutes: 60},
		"Approaching: 6 km closer in 30 min.":                          {Movement: "approaching", NearestChangeKm: km(-6), WindowMinutes: 30},
		"":                                                             {Movement: "unknown"},
	}
	for want, tr := range tests {
		assert.Equal(t, want, trendText(tr), tr.Movement)
	}
	assert.Equal(t, "", capitalize(""))
}
