//go:build integration

package e2e_test

import (
	"context"
	"testing"
	"time"

	"github.com/rubiojr/aemet-go/radar"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRadarIntegration checks AEMET's public radar endpoints, which need no
// API key, still return composites this package can read.
func TestRadarIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var c radar.Client

	legend, err := c.Legend(ctx)
	require.NoError(t, err)
	assert.Len(t, legend, 11, "AEMET's reflectivity legend changed")

	r, err := c.Check(ctx, 40.4168, -3.7038, radar.Options{}) // Madrid
	require.NoError(t, err)
	assert.Less(t, r.AgeMinutes, 180, "latest composite is stale")
	assert.NotEqual(t, radar.StatusNoCoverage, r.Latest.Status, "Madrid should be covered by radar")
	assert.Greater(t, r.Latest.CoveragePercent, 50.0)
	assert.NotEmpty(t, r.History)

	r, err = c.Check(ctx, 28.4636, -16.2518, radar.Options{NoHistory: true}) // Santa Cruz de Tenerife
	require.NoError(t, err)
	assert.Equal(t, radar.StatusNoCoverage, r.Latest.Status, "the composite does not include the Canary Islands")
}
