package weather

import (
	"fmt"
	"strings"
	"time"
	_ "time/tzdata" // AEMET dates are Spanish local dates; do not rely on system tzdata.

	"github.com/rubiojr/aemet-go"
)

const apiDate = "2006-01-02T15:04:05"

// Period is the forecast for part of a day.
type Period struct {
	Hours           string `json:"hours" jsonschema:"hour range in local Spanish time, e.g. 00-12, 12-24 or 00-24"`
	Sky             string `json:"sky,omitempty" jsonschema:"sky condition as reported by AEMET, in Spanish"`
	RainProbability int    `json:"rain_probability" jsonschema:"precipitation probability, percent"`
	WindDirection   string `json:"wind_direction,omitempty" jsonschema:"wind origin: N, NE, E, SE, S, SW, W, NW or C for calm"`
	WindSpeedKmh    int    `json:"wind_speed_kmh"`
}

// Day is the forecast for one date.
type Day struct {
	Date        string   `json:"date" jsonschema:"YYYY-MM-DD"`
	Weekday     string   `json:"weekday"`
	MinTempC    int      `json:"min_temp_c"`
	MaxTempC    int      `json:"max_temp_c"`
	MinFeelsC   int      `json:"min_feels_like_c"`
	MaxFeelsC   int      `json:"max_feels_like_c"`
	MinHumidity int      `json:"min_humidity"`
	MaxHumidity int      `json:"max_humidity"`
	UVMax       int      `json:"uv_max,omitempty"`
	Periods     []Period `json:"periods" jsonschema:"morning and afternoon when AEMET provides them, otherwise the whole day"`
}

// Forecast is a municipality's daily forecast.
type Forecast struct {
	Place  Place  `json:"place"`
	Issued string `json:"issued,omitempty" jsonschema:"when AEMET produced the forecast, local Spanish time"`
	Days   []Day  `json:"days"`
}

var madrid = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		return time.Local
	}
	return loc
}()

// Convert turns an AEMET API forecast into a Forecast for place, skipping
// days before today's date in Spain and keeping at most maxDays (0 for all).
func Convert(place Place, m *aemet.Municipality, now time.Time, maxDays int) Forecast {
	today := now.In(madrid).Format(time.DateOnly)
	f := Forecast{Place: place, Issued: m.Elaborado, Days: []Day{}}
	if place.Province == "" {
		f.Place.Province = m.Provincia
	}
	for _, d := range m.Prediccion.Dia {
		day := convertDay(d)
		if day.Date != "" && day.Date < today {
			continue
		}
		f.Days = append(f.Days, day)
		if maxDays > 0 && len(f.Days) == maxDays {
			break
		}
	}
	return f
}

func convertDay(d aemet.Dia) Day {
	day := Day{
		Date:        d.Fecha,
		MinTempC:    d.Temperatura.Minima,
		MaxTempC:    d.Temperatura.Maxima,
		MinFeelsC:   d.SensTermica.Minima,
		MaxFeelsC:   d.SensTermica.Maxima,
		MinHumidity: d.HumedadRelativa.Minima,
		MaxHumidity: d.HumedadRelativa.Maxima,
		UVMax:       d.UvMax,
		Periods:     periods(d),
	}
	if t, err := time.Parse(apiDate, d.Fecha); err == nil {
		day.Date = t.Format(time.DateOnly)
		day.Weekday = t.Weekday().String()
	}
	return day
}

// periods returns morning and afternoon when both are present, otherwise
// the whole day. AEMET omits the period on the last forecast days.
func periods(d aemet.Dia) []Period {
	byHours := map[string]*Period{}
	get := func(h string) *Period {
		if h == "" {
			h = "00-24"
		}
		if p, ok := byHours[h]; ok {
			return p
		}
		p := &Period{Hours: h}
		byHours[h] = p
		return p
	}
	for _, p := range d.ProbPrecipitacion {
		get(p.Periodo).RainProbability = p.Value
	}
	for _, s := range d.EstadoCielo {
		get(s.Periodo).Sky = s.Descripcion
	}
	for _, w := range d.Viento {
		p := get(w.Periodo)
		p.WindDirection, p.WindSpeedKmh = windDirection(w.Direccion), w.Velocidad
	}
	am, okAM := byHours["00-12"]
	pm, okPM := byHours["12-24"]
	if okAM && okPM {
		return []Period{*am, *pm}
	}
	if all, ok := byHours["00-24"]; ok {
		return []Period{*all}
	}
	return []Period{}
}

// windDirection translates AEMET's Spanish compass points (O, SO, NO) to English.
func windDirection(d string) string {
	switch d {
	case "O":
		return "W"
	case "SO":
		return "SW"
	case "NO":
		return "NW"
	}
	return d
}

// Fetch resolves query (a name or INE code) and returns its forecast.
func Fetch(c interface {
	GetForecastFor(string) (*aemet.Municipality, error)
}, query, province string, maxDays int) (Forecast, error) {
	place, err := Resolve(query, province)
	if err != nil {
		return Forecast{}, err
	}
	m, err := c.GetForecastFor(place.ID)
	if err != nil {
		return Forecast{}, fmt.Errorf("forecast for %s: %w", place.Name, err)
	}
	if m == nil {
		return Forecast{}, fmt.Errorf("forecast for %s: empty response", place.Name)
	}
	return Convert(place, m, time.Now(), maxDays), nil
}

// Headline picks the period that best summarises a day: the one with the
// highest rain probability, and the strongest wind.
func (d Day) Headline() Period {
	var h Period
	for i, p := range d.Periods {
		if i == 0 || p.RainProbability > h.RainProbability {
			h.Hours, h.Sky, h.RainProbability = p.Hours, p.Sky, p.RainProbability
		}
		if i == 0 || p.WindSpeedKmh > h.WindSpeedKmh {
			h.WindDirection, h.WindSpeedKmh = p.WindDirection, p.WindSpeedKmh
		}
	}
	return h
}

// WeatherEmoji returns an emoji for a sky description and rain probability.
func WeatherEmoji(sky string, rainProb int) string {
	sky = fold(sky)
	switch {
	case rainProb > 70:
		return "🌧️"
	case rainProb > 30:
		return "🌦️"
	case strings.Contains(sky, "tormenta"):
		return "⛈️"
	case strings.Contains(sky, "nieve"):
		return "❄️"
	case strings.Contains(sky, "niebla") || strings.Contains(sky, "bruma"):
		return "🌫️"
	case strings.Contains(sky, "lluvia"):
		if strings.Contains(sky, "escasa") {
			return "🌦️"
		}
		return "🌧️"
	case strings.Contains(sky, "poco nuboso"):
		return "🌤️"
	case strings.Contains(sky, "muy nuboso") || strings.Contains(sky, "cubierto"):
		return "☁️"
	case strings.Contains(sky, "nub"):
		return "⛅"
	default:
		return "☀️"
	}
}

// WindEmoji returns an arrow pointing where a wind from direction blows.
func WindEmoji(direction string) string {
	switch direction {
	case "N":
		return "⬇️"
	case "NE":
		return "↙️"
	case "E":
		return "⬅️"
	case "SE":
		return "↖️"
	case "S":
		return "⬆️"
	case "SW":
		return "↗️"
	case "W":
		return "➡️"
	case "NW":
		return "↘️"
	case "C":
		return "🔄"
	default:
		return "💨"
	}
}
