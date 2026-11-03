// Package util holds flags and helpers shared by the aemet commands.
package util

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"runtime/debug"
	"strings"

	"github.com/rubiojr/aemet-go"
	"github.com/urfave/cli/v3"
)

// AppName is the name of the CLI binary.
const AppName = "aemet"

// Version is set at build time with -ldflags "-X .../util.Version=v1.2.3".
// When unset it falls back to the module version recorded by go install/go run.
var Version = ""

// GetVersion returns the CLI version.
func GetVersion() string {
	if Version != "" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "dev"
}

// Forecaster fetches municipality forecasts. *aemet.Client implements it.
type Forecaster interface {
	GetForecastFor(id string) (*aemet.Municipality, error)
}

// NewClient builds a Forecaster for an API key. Tests replace it.
var NewClient = func(apiKey string) (Forecaster, error) {
	c, err := aemet.New(aemet.Config{
		AemetApiKey: apiKey,
		// The library logs request URLs, which include the API key.
		Logger: log.New(&RedactWriter{W: os.Stderr, Secret: apiKey}, "", log.LstdFlags),
	})
	if err != nil {
		return nil, Redact(err, apiKey)
	}
	return redactingClient{c: c, key: apiKey}, nil
}

// redactingClient strips the API key from errors, which can carry request URLs.
type redactingClient struct {
	c   Forecaster
	key string
}

func (r redactingClient) GetForecastFor(id string) (*aemet.Municipality, error) {
	m, err := r.c.GetForecastFor(id)
	return m, Redact(err, r.key)
}

// GlobalFlags returns the flags available to every command.
func GlobalFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:    "api-key",
			Usage:   "AEMET OpenData API key",
			Sources: cli.EnvVars(aemet.EnvAemetApiKey),
		},
	}
}

// APIKey returns the API key from the global flag or the environment.
func APIKey(cmd *cli.Command) string {
	return cmd.String("api-key")
}

// Client returns a Forecaster configured with the command's API key.
func Client(cmd *cli.Command) (Forecaster, error) {
	key := APIKey(cmd)
	if key == "" {
		return nil, fmt.Errorf("AEMET API key required: set %s or --api-key", aemet.EnvAemetApiKey)
	}
	c, err := NewClient(key)
	if err != nil {
		return nil, fmt.Errorf("create client: %w", Redact(err, key))
	}
	return c, nil
}

// Redact returns err with every occurrence of secret replaced.
func Redact(err error, secret string) error {
	if err == nil || secret == "" || !strings.Contains(err.Error(), secret) {
		return err
	}
	return redactedError{msg: strings.ReplaceAll(err.Error(), secret, "[REDACTED]"), err: err}
}

type redactedError struct {
	msg string
	err error
}

func (e redactedError) Error() string { return e.msg }
func (e redactedError) Unwrap() error { return e.err }

// RedactWriter replaces Secret in everything written to W. It assumes the
// secret is not split across writes, which holds for log.Logger output.
type RedactWriter struct {
	W      io.Writer
	Secret string
}

func (r *RedactWriter) Write(p []byte) (int, error) {
	if r.Secret == "" {
		return r.W.Write(p)
	}
	if _, err := r.W.Write(bytes.ReplaceAll(p, []byte(r.Secret), []byte("[REDACTED]"))); err != nil {
		return 0, err
	}
	return len(p), nil
}
