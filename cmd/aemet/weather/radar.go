package weather

import (
	"fmt"
	"strings"
	"time"

	"github.com/rubiojr/aemet-go/radar"
)

// RadarSummary renders a radar report around place as one English line.
func RadarSummary(place string, radiusKm float64, r radar.Report) string {
	l := r.Latest
	var b strings.Builder
	fmt.Fprintf(&b, "Radar %s (%d min old): ", localClock(l.Time), r.AgeMinutes)
	switch l.Status {
	case radar.StatusNoCoverage:
		fmt.Fprintf(&b, "no radar coverage at %s.", place)
		return b.String()
	case radar.StatusPrecipitation:
		fmt.Fprintf(&b, "%s precipitation at %s (%s).", capitalize(l.Here.Label), place, rate(*l.Here))
	default:
		fmt.Fprintf(&b, "dry at %s", place)
		if l.Nearest == nil {
			fmt.Fprintf(&b, ", no precipitation within %g km.", radiusKm)
		} else {
			fmt.Fprintf(&b, "; nearest precipitation %s, %g km %s.", l.Nearest.Label, l.Nearest.DistanceKm, l.Nearest.Direction)
		}
	}
	if s := l.Strongest; s != nil && (l.Here == nil || s.MinDBZ > l.Here.MinDBZ) && s.Direction != "here" {
		fmt.Fprintf(&b, " Strongest within %g km: %s (%s), %g km %s.", radiusKm, s.Label, rate(s.Intensity), s.DistanceKm, s.Direction)
	}
	if t := trendText(r.Trend); t != "" {
		b.WriteString(" " + t)
	}
	return b.String()
}

func trendText(t radar.Trend) string {
	w := t.WindowMinutes
	switch t.Movement {
	case "approaching":
		s := fmt.Sprintf("Approaching: %g km closer in %d min", -*t.NearestChangeKm, w)
		if t.EtaMinutes != nil {
			s += fmt.Sprintf(", could arrive in about %d min", *t.EtaMinutes)
		}
		return s + "."
	case "receding":
		return fmt.Sprintf("Moving away: %g km farther in %d min.", *t.NearestChangeKm, w)
	case "steady":
		return fmt.Sprintf("Nearest precipitation about the same distance as %d min ago.", w)
	case "arrived":
		return fmt.Sprintf("Started within the last %d min.", w)
	case "cleared":
		return fmt.Sprintf("Stopped within the last %d min.", w)
	case "ongoing":
		return fmt.Sprintf("Raining throughout the last %d min.", w)
	case "appeared":
		return fmt.Sprintf("Precipitation appeared nearby within the last %d min.", w)
	case "dissipated":
		return fmt.Sprintf("Nearby precipitation dissipated within the last %d min.", w)
	}
	return ""
}

func rate(in radar.Intensity) string {
	return fmt.Sprintf("%d-%d dBZ, ~%g-%g mm/h", in.MinDBZ, in.MaxDBZ, in.MinMmH, in.MaxMmH)
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// localClock formats t as Spanish peninsular local time.
func localClock(t time.Time) string {
	if madrid, err := time.LoadLocation("Europe/Madrid"); err == nil {
		t = t.In(madrid)
	}
	return t.Format("15:04 MST")
}
