package aemet

type WeatherStation struct {
	Latitude  string `json:"latitud"`
	Province  string `json:"provincia"`
	Altitude  string `json:"altitud"`
	ID        string `json:"indicativo"`
	Name      string `json:"nombre"`
	IndSinop  string `json:"indsinop"`
	Longitude string `json:"longitud"`
}

// ProbPrecipitacion represents precipitation probability data
type ProbPrecipitacion struct {
	Value   int    `json:"value"`
	Periodo string `json:"periodo"`
}

// CotaNieveProv represents snow level data
type CotaNieveProv struct {
	Value   string `json:"value"`
	Periodo string `json:"periodo"`
}

// EstadoCielo represents sky condition data
type EstadoCielo struct {
	Value       string `json:"value"`
	Periodo     string `json:"periodo"`
	Descripcion string `json:"descripcion"`
}

// Viento represents wind data
type Viento struct {
	Direccion string `json:"direccion"`
	Velocidad int    `json:"velocidad"`
	Periodo   string `json:"periodo"`
}

// RachaMax represents maximum wind gust data
type RachaMax struct {
	Value   string `json:"value"`
	Periodo string `json:"periodo"`
}

// Dato represents hourly data points
type Dato struct {
	Value int `json:"value"`
	Hora  int `json:"hora"`
}

// Temperatura represents temperature data
type Temperatura struct {
	Maxima int    `json:"maxima"`
	Minima int    `json:"minima"`
	Dato   []Dato `json:"dato"`
}

// Dia represents a day's forecast
type Dia struct {
	ProbPrecipitacion []ProbPrecipitacion `json:"probPrecipitacion"`
	CotaNieveProv     []CotaNieveProv     `json:"cotaNieveProv"`
	EstadoCielo       []EstadoCielo       `json:"estadoCielo"`
	Viento            []Viento            `json:"viento"`
	RachaMax          []RachaMax          `json:"rachaMax"`
	Temperatura       Temperatura         `json:"temperatura"`
	SensTermica       Temperatura         `json:"sensTermica"`
	HumedadRelativa   Temperatura         `json:"humedadRelativa"`
	UvMax             int                 `json:"uvMax"`
	Fecha             string              `json:"fecha"`
}

// Prediccion represents the prediction structure
type Prediccion struct {
	Dia []Dia `json:"dia"`
}

// Municipality represents a municipality forecast
type Municipality struct {
	Elaborado  string     `json:"elaborado"`
	Nombre     string     `json:"nombre"`
	Provincia  string     `json:"provincia"`
	Prediccion Prediccion `json:"prediccion"`
	ID         int        `json:"id"`
	Version    float64    `json:"version"`
}