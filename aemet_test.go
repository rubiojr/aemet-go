package aemet

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testClient returns a client whose API base URL is srv.
func testClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	c, err := New(Config{AemetApiKey: "secret", HTTPClient: srv.Client(), Logger: log.New(io.Discard, "", 0)})
	require.NoError(t, err)
	c.baseURL = srv.URL
	return c
}

func TestGetRedir(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "secret", r.URL.Query().Get("api_key"))
		_, _ = io.WriteString(w, `{"descripcion":"exito","estado":200,"datos":"`+srv.URL+`/data"}`)
	})
	mux.HandleFunc("/data", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `[{"nombre":"Madrid"}]`)
	})
	mux.HandleFunc("/limited", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"descripcion":"Límite de peticiones o caudal por minuto excedido para este usuario.","estado":429}`)
	})
	mux.HandleFunc("/gone", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"estado":200,"datos":"`+srv.URL+`/missing"}`)
	})
	mux.HandleFunc("/html", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, `<html>bad gateway</html>`)
	})
	c := testClient(t, srv)

	var got []Municipality
	require.NoError(t, c.getRedir("ok", &got))
	require.Len(t, got, 1)
	assert.Equal(t, "Madrid", got[0].Nombre)

	// AEMET errors carry no data URL; report AEMET's description instead of
	// requesting a malformed URL.
	err := c.getRedir("limited", &got)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 429")
	assert.Contains(t, err.Error(), "Límite de peticiones")
	assert.NotContains(t, err.Error(), "%!s")

	err = c.getRedir("gone", &got)
	assert.ErrorContains(t, err, "HTTP 404")

	err = c.getRedir("html", &got)
	assert.ErrorContains(t, err, "HTTP 502")
}
