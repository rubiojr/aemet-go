package weather

import (
	"fmt"
	"io"
	"strings"
	"time"
)

var periodNames = map[string]string{
	"00-12": "Morning (00-12h)",
	"12-24": "Afternoon (12-24h)",
	"00-24": "All day",
}

// FormatPeriod renders a period as one line without its name.
func FormatPeriod(p Period) string {
	var b strings.Builder
	b.WriteString(WeatherEmoji(p.Sky, p.RainProbability))
	if p.Sky != "" {
		b.WriteString(" " + p.Sky)
	}
	if p.RainProbability > 0 {
		fmt.Fprintf(&b, " (💧 %d%%)", p.RainProbability)
	}
	if p.WindDirection != "" && p.WindSpeedKmh > 0 {
		fmt.Fprintf(&b, " %s %s at %d km/h", WindEmoji(p.WindDirection), p.WindDirection, p.WindSpeedKmh)
	}
	return b.String()
}

// DayTitle renders a day's date and temperature range.
func DayTitle(d Day) string {
	date := d.Date
	if t, err := time.Parse(time.DateOnly, d.Date); err == nil {
		date = t.Format("Monday, Jan 02")
	}
	return fmt.Sprintf("📅 %s (🌡️ %d°C to %d°C)", date, d.MinTempC, d.MaxTempC)
}

// WriteForecast renders a multi-day forecast.
func WriteForecast(w io.Writer, f Forecast) error {
	var b strings.Builder
	fmt.Fprintf(&b, "🌤️  Weather forecast for %s (%s)\n", f.Place.Name, f.Place.Province)
	if t, err := time.Parse(apiDate, f.Issued); err == nil {
		fmt.Fprintf(&b, "📊 Issued by AEMET on %s\n", t.Format("Monday, January 02 at 15:04"))
	}
	b.WriteString("==============================================\n")
	if len(f.Days) == 0 {
		b.WriteString("\nNo forecast days available.\n")
	}
	for _, d := range f.Days {
		b.WriteString("\n" + DayTitle(d) + "\n")
		for _, p := range d.Periods {
			name := periodNames[p.Hours]
			if name == "" {
				name = p.Hours
			}
			fmt.Fprintf(&b, "%s: %s\n", name, FormatPeriod(p))
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// Summary renders the first forecast day as one line.
func Summary(f Forecast) (string, error) {
	if len(f.Days) == 0 {
		return "", fmt.Errorf("no forecast data available for %s", f.Place.Name)
	}
	d := f.Days[0]
	h := d.Headline()
	s := fmt.Sprintf("%s %s: %s %d°C-%d°C", WeatherEmoji(h.Sky, h.RainProbability), f.Place.Name, h.Sky, d.MinTempC, d.MaxTempC)
	if h.RainProbability > 0 {
		s += fmt.Sprintf(" (💧 %d%%)", h.RainProbability)
	}
	if h.WindDirection != "" && h.WindSpeedKmh > 0 {
		s += fmt.Sprintf(" %s %d km/h", WindEmoji(h.WindDirection), h.WindSpeedKmh)
	}
	return s, nil
}
