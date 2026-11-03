// Package fake provides AEMET test doubles shared by the command tests.
package fake

import (
	"sync"
	"time"

	"github.com/rubiojr/aemet-go"
)

// Client is a Forecaster returning a canned response and recording calls.
type Client struct {
	M   *aemet.Municipality
	Err error

	mu  sync.Mutex
	IDs []string
}

// GetForecastFor records id and returns the canned response.
func (c *Client) GetForecastFor(id string) (*aemet.Municipality, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.IDs = append(c.IDs, id)
	return c.M, c.Err
}

func dateOffset(now time.Time, days int) string {
	madrid, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		madrid = time.Local
	}
	return now.In(madrid).AddDate(0, 0, days).Format(time.DateOnly) + "T00:00:00"
}

// Sample returns an API response, relative to now, with yesterday, today
// split into morning and afternoon, and tomorrow without periods.
func Sample(now time.Time) *aemet.Municipality {
	return &aemet.Municipality{
		Elaborado: "2026-11-03T08:00:00",
		Nombre:    "Madrid",
		Provincia: "Madrid",
		Prediccion: aemet.Prediccion{Dia: []aemet.Dia{
			{Fecha: dateOffset(now, -1), Temperatura: aemet.Temperatura{Minima: 1, Maxima: 2}},
			{
				Fecha: dateOffset(now, 0),
				ProbPrecipitacion: []aemet.ProbPrecipitacion{
					{Value: 80, Periodo: "00-24"}, {Value: 10, Periodo: "00-12"}, {Value: 40, Periodo: "12-24"},
				},
				EstadoCielo: []aemet.EstadoCielo{
					{Descripcion: "Poco nuboso", Periodo: "00-12"}, {Descripcion: "Nuboso con lluvia", Periodo: "12-24"},
				},
				Viento: []aemet.Viento{
					{Direccion: "NO", Velocidad: 25, Periodo: "00-12"}, {Direccion: "S", Velocidad: 10, Periodo: "12-24"},
				},
				Temperatura:     aemet.Temperatura{Minima: 8, Maxima: 17},
				SensTermica:     aemet.Temperatura{Minima: 6, Maxima: 16},
				HumedadRelativa: aemet.Temperatura{Minima: 40, Maxima: 90},
				UvMax:           3,
			},
			{
				Fecha:             dateOffset(now, 1),
				ProbPrecipitacion: []aemet.ProbPrecipitacion{{Value: 0}},
				EstadoCielo:       []aemet.EstadoCielo{{Descripcion: "Despejado"}},
				Viento:            []aemet.Viento{{Direccion: "C", Velocidad: 0}},
				Temperatura:       aemet.Temperatura{Minima: 5, Maxima: 20},
			},
		}},
	}
}
