// Package radartest serves recorded AEMET radar responses for tests.
//
// The fixtures are the national composites published from 22:40 to 23:50
// UTC on 2026-11-03, while a storm crossed the Barcelona coast, plus the
// legend, image bounds and a timeline trimmed to those frames. The
// timeline also holds an entry with a bad date and one with an unsafe file
// name, which clients must ignore.
package radartest

import (
	"embed"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

//go:embed testdata
var files embed.FS

// Latest is the time of the newest fixture composite.
var Latest = time.Date(2026, 11, 3, 23, 50, 0, 0, time.UTC)

// Server is a fake AEMET radar API.
type Server struct {
	*httptest.Server
	requests atomic.Int64

	mu   sync.Mutex
	fail map[string]bool
}

// NewServer starts a Server that is closed when the test ends. Its URL is
// a radar.Client BaseURL.
func NewServer(t testing.TB) *Server {
	t.Helper()
	s := &Server{fail: map[string]bool{}}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

// Fail makes requests for the composite file name return HTTP 500.
func (s *Server) Fail(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fail[name] = true
}

// Requests returns how many requests the server has received.
func (s *Server) Requests() int64 {
	return s.requests.Load()
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	s.requests.Add(1)
	p := strings.TrimPrefix(r.URL.Path, "/")
	var name string
	switch {
	case p == "timeline/compo/PB":
		name = "timeline.json"
	case p == "leyenda-radar/compo":
		name = "legend.json"
	case strings.HasPrefix(p, "bounds-radar/compo/"):
		name = "bounds.json"
	case strings.HasPrefix(p, "imagen-radar/compo/"):
		name = strings.TrimPrefix(p, "imagen-radar/compo/")
		s.mu.Lock()
		failed := s.fail[name]
		s.mu.Unlock()
		if failed {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
	}
	if name == "" || strings.Contains(name, "/") {
		http.NotFound(w, r)
		return
	}
	http.ServeFileFS(w, r, files, "testdata/"+name)
}
