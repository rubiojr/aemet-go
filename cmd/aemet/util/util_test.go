package util

import (
	"errors"
	"strings"
	"testing"

	"github.com/rubiojr/aemet-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stub struct{ err error }

func (s stub) GetForecastFor(string) (*aemet.Municipality, error) { return nil, s.err }

func TestRedact(t *testing.T) {
	base := errors.New("GET ?api_key=k3y failed")
	err := Redact(base, "k3y")
	assert.Equal(t, "GET ?api_key=[REDACTED] failed", err.Error())
	assert.ErrorIs(t, err, base)

	assert.Same(t, base, Redact(base, ""))
	assert.Same(t, base, Redact(base, "other"))
	assert.NoError(t, Redact(nil, "k3y"))
}

func TestRedactWriter(t *testing.T) {
	var b strings.Builder
	w := &RedactWriter{W: &b, Secret: "k3y"}
	n, err := w.Write([]byte("url?api_key=k3y\n"))
	require.NoError(t, err)
	assert.Equal(t, 16, n)
	assert.Equal(t, "url?api_key=[REDACTED]\n", b.String())

	b.Reset()
	_, err = (&RedactWriter{W: &b}).Write([]byte("plain"))
	require.NoError(t, err)
	assert.Equal(t, "plain", b.String())
}

func TestRedactingClient(t *testing.T) {
	c := redactingClient{c: stub{errors.New("api_key=k3y")}, key: "k3y"}
	_, err := c.GetForecastFor("28079")
	assert.EqualError(t, err, "api_key=[REDACTED]")

	fc, err := NewClient("k3y")
	require.NoError(t, err)
	assert.IsType(t, redactingClient{}, fc)
}

func TestGetVersion(t *testing.T) {
	assert.NotEmpty(t, GetVersion())
	orig := Version
	t.Cleanup(func() { Version = orig })
	Version = "v9.9.9"
	assert.Equal(t, "v9.9.9", GetVersion())
}
