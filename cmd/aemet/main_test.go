package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rubiojr/aemet-go"
	"github.com/rubiojr/aemet-go/cmd/aemet/internal/fake"
	radarcmd "github.com/rubiojr/aemet-go/cmd/aemet/radar"
	"github.com/rubiojr/aemet-go/cmd/aemet/util"
	"github.com/rubiojr/aemet-go/cmd/aemet/weather"
	"github.com/rubiojr/aemet-go/internal/radartest"
	"github.com/rubiojr/aemet-go/radar"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testKey = "secret-key-123"

type result struct {
	out, errOut string
	err         error
}

// run executes the CLI with a fake AEMET client and returns its output.
func run(t *testing.T, client *fake.Client, stdin string, args ...string) result {
	t.Helper()
	t.Setenv(aemet.EnvAemetApiKey, testKey)
	orig := util.NewClient
	util.NewClient = func(key string) (util.Forecaster, error) {
		assert.Equal(t, testKey, key)
		return client, nil
	}
	t.Cleanup(func() { util.NewClient = orig })

	var out, errOut strings.Builder
	app := newApp()
	app.Writer, app.ErrWriter, app.Reader = &out, &errOut, strings.NewReader(stdin)
	err := app.Run(context.Background(), append([]string{"aemet"}, args...))
	return result{out.String(), errOut.String(), err}
}

func sample() *fake.Client {
	return &fake.Client{M: fake.Sample(time.Now())}
}

func TestForecast(t *testing.T) {
	c := sample()
	r := run(t, c, "", "forecast", "Madrid")
	require.NoError(t, r.err)
	assert.Equal(t, []string{"28079"}, c.IDs)
	assert.Contains(t, r.out, "Weather forecast for Madrid (Madrid)")
	assert.Contains(t, r.out, "Morning (00-12h)")

	r = run(t, sample(), "", "f", "-n", "a coruña", "--days", "1", "--json")
	require.NoError(t, r.err)
	var f weather.Forecast
	require.NoError(t, json.Unmarshal([]byte(r.out), &f))
	assert.Equal(t, "15030", f.Place.ID)
	assert.Len(t, f.Days, 1)
}

func TestForecastAmbiguous(t *testing.T) {
	c := sample()
	r := run(t, c, "2\n", "forecast", "Mieres")
	require.NoError(t, r.err)
	assert.Contains(t, r.errOut, "Select a municipality (1-2)")
	require.Len(t, c.IDs, 1)
	assert.Equal(t, "17105", c.IDs[0], "second candidate, sorted by population")

	r = run(t, sample(), "", "forecast", "-i", "Mieres")
	require.NoError(t, r.err)
	assert.Contains(t, r.errOut, "using Mieres (Asturias")

	for _, in := range []string{"9\n", "x\n", ""} {
		r = run(t, sample(), in, "forecast", "Mieres")
		assert.Error(t, r.err, "input %q", in)
	}

	c = sample()
	r = run(t, c, "", "forecast", "-p", "girona", "Mieres")
	require.NoError(t, r.err)
	assert.Equal(t, []string{"17105"}, c.IDs)
}

func TestForecastErrors(t *testing.T) {
	assert.ErrorContains(t, run(t, sample(), "", "forecast").err, "required")
	assert.ErrorContains(t, run(t, sample(), "", "forecast", "-n", "x", "y").err, "not both")
	assert.ErrorContains(t, run(t, sample(), "", "forecast", "--days", "-1", "Madrid").err, "negative")
	assert.ErrorContains(t, run(t, sample(), "", "forecast", "Nowhereville").err, "no municipality")

	r := run(t, &fake.Client{Err: errors.New("GET https://x?api_key=" + testKey + " failed")}, "", "forecast", "Madrid")
	require.Error(t, r.err)
	assert.NotContains(t, r.err.Error(), testKey)
	assert.Contains(t, r.err.Error(), "[REDACTED]")
}

func TestMissingAPIKey(t *testing.T) {
	t.Setenv(aemet.EnvAemetApiKey, "")
	app := newApp()
	var out strings.Builder
	app.Writer, app.ErrWriter = &out, &out
	err := app.Run(context.Background(), []string{"aemet", "day", "Madrid"})
	assert.ErrorContains(t, err, "AEMET_API_KEY")
}

func TestDay(t *testing.T) {
	c := sample()
	r := run(t, c, "", "day", "Madrid", "-c", "Bilbao,28079")
	require.NoError(t, r.err)
	assert.Equal(t, []string{"48020", "28079", "28079"}, c.IDs)
	assert.Contains(t, r.out, "El tiempo hoy")
	assert.Equal(t, 3, strings.Count(r.out, "8°C-17°C"))

	r = run(t, sample(), "", "day", "--json", "Palma")
	require.NoError(t, r.err)
	var fs []weather.Forecast
	require.NoError(t, json.Unmarshal([]byte(r.out), &fs))
	require.Len(t, fs, 1)
	assert.Equal(t, "07040", fs[0].Place.ID, "ambiguous names use the best match")
	assert.Len(t, fs[0].Days, 1)

	r = run(t, sample(), "", "day", "--use-ids", "Madrid", "28079", "Nowhereville")
	assert.ErrorContains(t, r.err, "2 of 3")
	assert.Contains(t, r.errOut, "Madrid: not a 5-digit")
	assert.Contains(t, r.errOut, "Nowhereville: not a 5-digit")
	assert.Equal(t, 1, strings.Count(r.out, "8°C-17°C"))

	r = run(t, &fake.Client{M: &aemet.Municipality{}}, "", "day", "Madrid")
	assert.Error(t, r.err)
	assert.Contains(t, r.errOut, "no forecast data")

	assert.ErrorContains(t, run(t, sample(), "", "day").err, "at least one")
}

func TestSearch(t *testing.T) {
	r := run(t, nil, "", "search", "coruña")
	require.NoError(t, r.err)
	assert.True(t, strings.HasPrefix(r.out, "15030\tCoruña, A\t"))

	r = run(t, nil, "", "s", "--json", "--limit", "1", "valencia")
	require.NoError(t, r.err)
	var ps []weather.Place
	require.NoError(t, json.Unmarshal([]byte(r.out), &ps))
	assert.Equal(t, []weather.Place{{ID: "46250", Name: "Valencia", Province: "Valencia/València", Population: ps[0].Population}}, ps)

	r = run(t, nil, "", "search", "--json", "zzzz")
	require.NoError(t, r.err)
	assert.Equal(t, "[]\n", r.out)

	assert.Error(t, run(t, nil, "", "search", "zzzz").err)
	assert.Error(t, run(t, nil, "", "search").err)
}

func TestRadar(t *testing.T) {
	srv := radartest.NewServer(t)
	orig := radarcmd.Client
	radarcmd.Client = &radar.Client{BaseURL: srv.URL}
	t.Cleanup(func() { radarcmd.Client = orig })
	t.Setenv(aemet.EnvAemetApiKey, "")

	r := run(t, nil, "", "radar", "El", "Masnou")
	require.NoError(t, r.err)
	assert.Contains(t, r.out, "Moderate precipitation at Masnou, El")

	r = run(t, nil, "", "r", "--json", "--radius", "10", "--history", "0", "28079")
	require.NoError(t, r.err)
	var out radarcmd.Output
	require.NoError(t, json.Unmarshal([]byte(r.out), &out))
	assert.Equal(t, "28079", out.Place.ID)
	assert.Equal(t, radar.StatusDry, out.Latest.Status)
	assert.Equal(t, 10.0, out.RadiusKm)
	assert.Empty(t, out.History)
	assert.NotEmpty(t, out.Summary)

	r = run(t, nil, "", "radar", "Mieres")
	require.NoError(t, r.err)
	assert.Contains(t, r.errOut, "using Mieres")

	assert.ErrorContains(t, run(t, nil, "", "radar").err, "required")
	assert.ErrorContains(t, run(t, nil, "", "radar", "--radius", "0", "Madrid").err, "--radius")
	assert.ErrorContains(t, run(t, nil, "", "radar", "--radius", "500", "Madrid").err, "radius")
	assert.ErrorContains(t, run(t, nil, "", "radar", "--history", "999", "Madrid").err, "history")
	assert.ErrorContains(t, run(t, nil, "", "radar", "Nowhereville").err, "no municipality")

	srv.Fail("radw202611032350_3857.png")
	assert.ErrorContains(t, run(t, nil, "", "radar", "Madrid").err, "HTTP 500")
}

func TestVersion(t *testing.T) {
	r := run(t, nil, "", "version")
	require.NoError(t, r.err)
	assert.True(t, strings.HasPrefix(r.out, "aemet version "))
}
