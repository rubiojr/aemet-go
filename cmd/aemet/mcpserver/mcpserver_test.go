package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rubiojr/aemet-go/cmd/aemet/internal/fake"
	"github.com/rubiojr/aemet-go/cmd/aemet/util"
	"github.com/rubiojr/aemet-go/cmd/aemet/weather"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func connect(t *testing.T, newClient func() (util.Forecaster, error)) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	ss, err := NewServer(newClient, nil).Connect(ctx, st, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func call(t *testing.T, cs *mcp.ClientSession, tool string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
	require.NoError(t, err)
	return res
}

func text(res *mcp.CallToolResult) string {
	if len(res.Content) == 0 {
		return ""
	}
	if tc, ok := res.Content[0].(*mcp.TextContent); ok {
		return tc.Text
	}
	return ""
}

func decode[T any](t *testing.T, res *mcp.CallToolResult) T {
	t.Helper()
	require.False(t, res.IsError, text(res))
	b, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	var v T
	require.NoError(t, json.Unmarshal(b, &v))
	return v
}

func TestListTools(t *testing.T) {
	cs := connect(t, nil)
	res, err := cs.ListTools(context.Background(), nil)
	require.NoError(t, err)
	names := map[string]*mcp.Tool{}
	for _, tool := range res.Tools {
		names[tool.Name] = tool
	}
	require.Contains(t, names, "get_forecast")
	require.Contains(t, names, "search_municipalities")
	assert.True(t, names["get_forecast"].Annotations.ReadOnlyHint)
	assert.NotNil(t, names["get_forecast"].OutputSchema)
	assert.Contains(t, cs.InitializeResult().Instructions, "AEMET")
}

func TestGetForecast(t *testing.T) {
	c := &fake.Client{M: fake.Sample(time.Now())}
	calls := 0
	cs := connect(t, func() (util.Forecaster, error) { calls++; return c, nil })

	f := decode[weather.Forecast](t, call(t, cs, "get_forecast", map[string]any{"location": "a coruña"}))
	assert.Equal(t, "15030", f.Place.ID)
	assert.Len(t, f.Days, 2)
	assert.Equal(t, "NW", f.Days[0].Periods[0].WindDirection)

	f = decode[weather.Forecast](t, call(t, cs, "get_forecast", map[string]any{"location": "Mieres", "province": "Asturias", "days": 1}))
	assert.Equal(t, "33037", f.Place.ID)
	assert.Len(t, f.Days, 1)
	assert.Equal(t, []string{"15030", "33037"}, c.IDs)
	assert.Equal(t, 2, calls)
}

func TestGetForecastErrors(t *testing.T) {
	cs := connect(t, func() (util.Forecaster, error) {
		return &fake.Client{Err: errors.New("timeout")}, nil
	})
	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{"ambiguous", map[string]any{"location": "Mieres"}, "33037"},
		{"unknown", map[string]any{"location": "Nowhereville"}, "no municipality"},
		{"days too high", map[string]any{"location": "Madrid", "days": 8}, "between 1 and 7"},
		{"days negative", map[string]any{"location": "Madrid", "days": -1}, "between 1 and 7"},
		{"long", map[string]any{"location": string(make([]byte, 200))}, "too long"},
		{"upstream", map[string]any{"location": "Madrid"}, "try again later: forecast for Madrid: timeout"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := call(t, cs, "get_forecast", tt.args)
			assert.True(t, res.IsError)
			assert.Contains(t, text(res), tt.want)
		})
	}

	res := call(t, cs, "get_forecast", map[string]any{})
	assert.True(t, res.IsError, "location is required by the schema")
}

func TestNoAPIKey(t *testing.T) {
	cs := connect(t, lazyClient(""))
	res := call(t, cs, "get_forecast", map[string]any{"location": "Madrid"})
	assert.True(t, res.IsError)
	assert.Contains(t, text(res), "AEMET_API_KEY")

	out := decode[SearchOutput](t, call(t, cs, "search_municipalities", map[string]any{"query": "madrid"}))
	assert.NotEmpty(t, out.Municipalities, "search works without a key")
}

func TestLazyClient(t *testing.T) {
	orig := util.NewClient
	t.Cleanup(func() { util.NewClient = orig })
	n := 0
	util.NewClient = func(key string) (util.Forecaster, error) {
		n++
		if key == "bad" {
			return nil, errors.New("bad key " + key)
		}
		return &fake.Client{}, nil
	}

	get := lazyClient("good")
	c1, err := get()
	require.NoError(t, err)
	c2, err := get()
	require.NoError(t, err)
	assert.Same(t, c1, c2)
	assert.Equal(t, 1, n)

	_, err = lazyClient("bad")()
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "bad key bad")
}

func TestSearchMunicipalities(t *testing.T) {
	cs := connect(t, nil)
	out := decode[SearchOutput](t, call(t, cs, "search_municipalities", map[string]any{"query": "valencia", "limit": 2}))
	require.Len(t, out.Municipalities, 2)
	assert.Equal(t, "46250", out.Municipalities[0].ID)

	out = decode[SearchOutput](t, call(t, cs, "search_municipalities", map[string]any{"query": "zzzz"}))
	assert.NotNil(t, out.Municipalities)
	assert.Empty(t, out.Municipalities)

	for _, args := range []map[string]any{
		{"query": "x", "limit": 51},
		{"query": "x", "limit": -1},
		{"query": ""},
	} {
		assert.True(t, call(t, cs, "search_municipalities", args).IsError, "%v", args)
	}
}
