//go:build integration

package e2e_test

import (
	"io"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rubiojr/aemet-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration(t *testing.T) {
	apiKey := os.Getenv(aemet.EnvAemetApiKey)
	if apiKey == "" {
		t.Skip("set AEMET_API_KEY to run the live AEMET integration test")
	}

	// Exercise Go's default TLS behavior without compatibility overrides.
	t.Setenv("GODEBUG", "")
	client, err := aemet.New(aemet.Config{
		// Retry errors can contain the API key in request URLs. Suppress those
		// logs and redact the returned error before reporting a test failure.
		Logger: log.New(io.Discard, "", 0),
	})
	require.NoError(t, err)

	forecast, err := client.GetForecastFor("28079")
	if err != nil {
		require.FailNow(t, "fetch Madrid forecast", "%s", strings.ReplaceAll(err.Error(), apiKey, "[REDACTED]"))
	}
	require.NotNil(t, forecast)
	assert.Equal(t, 28079, forecast.ID)
	assert.Equal(t, "Madrid", forecast.Nombre)
	assert.Equal(t, "Madrid", forecast.Provincia)
	_, err = time.Parse("2006-01-02T15:04:05", forecast.Elaborado)
	assert.NoError(t, err, "forecast must have a valid publication timestamp")
	require.NotEmpty(t, forecast.Prediccion.Dia, "forecast must contain daily predictions")

	for _, day := range forecast.Prediccion.Dia {
		_, err := time.Parse("2006-01-02T15:04:05", day.Fecha)
		assert.NoError(t, err, "forecast day must have a valid date")
		assert.GreaterOrEqual(t, day.Temperatura.Maxima, day.Temperatura.Minima,
			"maximum temperature must be at least the minimum for %s", day.Fecha)
		assert.NotEmpty(t, day.EstadoCielo, "forecast must contain sky conditions for %s", day.Fecha)
	}
}
