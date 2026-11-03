package weather

import (
	"errors"
	"testing"
	"time"

	"github.com/rubiojr/aemet-go"
	"github.com/rubiojr/aemet-go/cmd/aemet/internal/fake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolve(t *testing.T) {
	tests := []struct {
		name, query, province, wantID string
	}{
		{"exact", "Madrid", "", "28079"},
		{"id", "28079", "", "28079"},
		{"case and accents", "CADIZ", "", "11012"},
		{"article moved to front", "A Coruña", "", "15030"},
		{"aemet order", "Coruña, A", "", "15030"},
		{"apostrophe article", "L'Hospitalet de Llobregat", "", "08101"},
		{"slash alternate", "San Sebastián", "", "20069"},
		{"other slash alternate", "donostia", "", "20069"},
		{"unique exact beats prefixes", "Valencia", "", "46250"},
		{"unique partial", "Vitoria", "", "01059"},
		{"province disambiguates", "Mieres", "asturias", "33037"},
		{"province alternate name", "Valencia", "València", "46250"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := Resolve(tt.query, tt.province)
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, p.ID)
			assert.NotEmpty(t, p.Province)
		})
	}
}

func TestResolveErrors(t *testing.T) {
	_, err := Resolve("Mieres", "")
	var amb *AmbiguousError
	require.ErrorAs(t, err, &amb, "two exact matches")
	assert.Len(t, amb.Candidates, 2)
	assert.Contains(t, err.Error(), "33037")

	_, err = Resolve("Palma", "")
	require.ErrorAs(t, err, &amb, "only prefixes")
	assert.LessOrEqual(t, len(amb.Candidates), maxCandidates)
	assert.Equal(t, "07040", amb.Candidates[0].ID, "largest town first")

	_, err = Resolve("Nowhereville", "")
	assert.ErrorContains(t, err, "no municipality matches")
	_, err = Resolve("Madrid", "Cádiz")
	assert.ErrorContains(t, err, "in province")
	_, err = Resolve("  ", "")
	assert.ErrorContains(t, err, "required")
	_, err = Resolve(string(make([]byte, MaxQueryLen+1)), "")
	assert.ErrorContains(t, err, "too long")
	_, err = Resolve("99999", "")
	assert.ErrorContains(t, err, "unknown municipality id")
	_, err = Lookup("28")
	assert.ErrorContains(t, err, "invalid municipality id")
	assert.False(t, errors.As(err, &amb))
}

func TestSearch(t *testing.T) {
	places, err := Search("valencia", "", 3)
	require.NoError(t, err)
	require.Len(t, places, 3)
	assert.Equal(t, "Valencia", places[0].Name)

	places, err = Search("zzzz", "", 0)
	require.NoError(t, err)
	assert.Empty(t, places)

	_, err = Search("", "", 0)
	assert.Error(t, err)
}

func TestConvert(t *testing.T) {
	now := time.Now()
	place, err := Lookup("28079")
	require.NoError(t, err)
	f := Convert(place, fake.Sample(now), now, 0)

	require.Len(t, f.Days, 2, "yesterday is dropped")
	today := f.Days[0]
	assert.Equal(t, now.In(madrid).Format(time.DateOnly), today.Date)
	assert.NotEmpty(t, today.Weekday)
	assert.Equal(t, 17, today.MaxTempC)
	assert.Equal(t, 6, today.MinFeelsC)
	assert.Equal(t, 90, today.MaxHumidity)
	assert.Equal(t, []Period{
		{Hours: "00-12", Sky: "Poco nuboso", RainProbability: 10, WindDirection: "NW", WindSpeedKmh: 25},
		{Hours: "12-24", Sky: "Nuboso con lluvia", RainProbability: 40, WindDirection: "S", WindSpeedKmh: 10},
	}, today.Periods)
	assert.Equal(t, []Period{{Hours: "00-24", Sky: "Despejado", WindDirection: "C"}}, f.Days[1].Periods)

	assert.Len(t, Convert(place, fake.Sample(now), now, 1).Days, 1)

	h := today.Headline()
	assert.Equal(t, "Nuboso con lluvia", h.Sky)
	assert.Equal(t, 40, h.RainProbability)
	assert.Equal(t, "NW", h.WindDirection)
	assert.Equal(t, 25, h.WindSpeedKmh)

	empty := Convert(Place{Name: "X"}, &aemet.Municipality{Provincia: "P"}, now, 0)
	assert.Equal(t, "P", empty.Place.Province)
	assert.NotNil(t, empty.Days)
}

func TestFetch(t *testing.T) {
	c := &fake.Client{M: fake.Sample(time.Now())}
	f, err := Fetch(c, "a coruna", "", 0)
	require.NoError(t, err)
	assert.Equal(t, []string{"15030"}, c.IDs)
	assert.Equal(t, "Coruña, A", f.Place.Name)

	_, err = Fetch(&fake.Client{Err: errors.New("boom")}, "Madrid", "", 0)
	assert.ErrorContains(t, err, "forecast for Madrid: boom")
	_, err = Fetch(&fake.Client{}, "Madrid", "", 0)
	assert.ErrorContains(t, err, "empty response")
	_, err = Fetch(&fake.Client{}, "Nowhereville", "", 0)
	assert.ErrorContains(t, err, "no municipality")
}
