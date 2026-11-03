package weather

import (
	"strings"
	"testing"
	"time"

	"github.com/rubiojr/aemet-go/cmd/aemet/internal/fake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteForecast(t *testing.T) {
	now := time.Now()
	place, err := Lookup("28079")
	require.NoError(t, err)
	f := Convert(place, fake.Sample(now), now, 0)

	var b strings.Builder
	require.NoError(t, WriteForecast(&b, f))
	out := b.String()
	assert.Contains(t, out, "Weather forecast for Madrid (Madrid)")
	assert.Contains(t, out, "Issued by AEMET on Tuesday, November 03 at 08:00")
	assert.Contains(t, out, "(🌡️ 8°C to 17°C)")
	assert.Contains(t, out, "Morning (00-12h): 🌤️ Poco nuboso (💧 10%) ↘️ NW at 25 km/h")
	assert.Contains(t, out, "Afternoon (12-24h): 🌦️ Nuboso con lluvia (💧 40%) ⬆️ S at 10 km/h")
	assert.Contains(t, out, "All day: ☀️ Despejado\n")

	b.Reset()
	require.NoError(t, WriteForecast(&b, Forecast{Place: place}))
	assert.Contains(t, b.String(), "No forecast days available")

	b.Reset()
	require.NoError(t, WriteForecast(&b, Forecast{Days: []Day{{Date: "bad", Periods: []Period{{Hours: "06-12"}}}}}))
	assert.Contains(t, b.String(), "📅 bad")
	assert.Contains(t, b.String(), "06-12: ☀️")
}

func TestSummary(t *testing.T) {
	now := time.Now()
	place, err := Lookup("28079")
	require.NoError(t, err)
	s, err := Summary(Convert(place, fake.Sample(now), now, 0))
	require.NoError(t, err)
	assert.Equal(t, "🌦️ Madrid: Nuboso con lluvia 8°C-17°C (💧 40%) ↘️ 25 km/h", s)

	_, err = Summary(Forecast{Place: place})
	assert.ErrorContains(t, err, "no forecast data")
}

func TestEmojis(t *testing.T) {
	sky := map[string]string{
		"Tormenta":                   "⛈️",
		"Nieve":                      "❄️",
		"Niebla":                     "🌫️",
		"Bruma":                      "🌫️",
		"Nuboso con lluvia":          "🌧️",
		"Cubierto con lluvia escasa": "🌦️",
		"Poco nuboso":                "🌤️",
		"Muy nuboso":                 "☁️",
		"Cubierto":                   "☁️",
		"Intervalos nubosos":         "⛅",
		"Despejado":                  "☀️",
	}
	for desc, want := range sky {
		assert.Equal(t, want, WeatherEmoji(desc, 0), desc)
	}
	assert.Equal(t, "🌧️", WeatherEmoji("Despejado", 71))
	assert.Equal(t, "🌦️", WeatherEmoji("Despejado", 31))

	for dir, want := range map[string]string{
		"N": "⬇️", "NE": "↙️", "E": "⬅️", "SE": "↖️", "S": "⬆️",
		"SW": "↗️", "W": "➡️", "NW": "↘️", "C": "🔄", "?": "💨",
	} {
		assert.Equal(t, want, WindEmoji(dir), dir)
	}
	for in, want := range map[string]string{"O": "W", "SO": "SW", "NO": "NW", "N": "N"} {
		assert.Equal(t, want, windDirection(in))
	}
}
