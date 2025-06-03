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

// Municipality_ProbPrecipitacion represents precipitation probability data
type Municipality_ProbPrecipitacion struct {
	Value   int    `json:"value"`
	Periodo string `json:"periodo"`
}

// Municipality_CotaNieveProv represents snow level data
type Municipality_CotaNieveProv struct {
	Value   string `json:"value"`
	Periodo string `json:"periodo"`
}

// Municipality_EstadoCielo represents sky condition data
type Municipality_EstadoCielo struct {
	Value       string `json:"value"`
	Periodo     string `json:"periodo"`
	Descripcion string `json:"descripcion"`
}

// Municipality_Viento represents wind data
type Municipality_Viento struct {
	Direccion string `json:"direccion"`
	Velocidad int    `json:"velocidad"`
	Periodo   string `json:"periodo"`
}

// Municipality_RachaMax represents maximum wind gust data
type Municipality_RachaMax struct {
	Value   string `json:"value"`
	Periodo string `json:"periodo"`
}

// Municipality_Dato represents hourly data points
type Municipality_Dato struct {
	Value int `json:"value"`
	Hora  int `json:"hora"`
}

// Municipality_Temperatura represents temperature data
type Municipality_Temperatura struct {
	Maxima int                `json:"maxima"`
	Minima int                `json:"minima"`
	Dato   []Municipality_Dato `json:"dato"`
}

// Municipality_Dia represents a day's forecast
type Municipality_Dia struct {
	ProbPrecipitacion []Municipality_ProbPrecipitacion `json:"probPrecipitacion"`
	CotaNieveProv     []Municipality_CotaNieveProv     `json:"cotaNieveProv"`
	EstadoCielo       []Municipality_EstadoCielo       `json:"estadoCielo"`
	Viento            []Municipality_Viento            `json:"viento"`
	RachaMax          []Municipality_RachaMax          `json:"rachaMax"`
	Temperatura       Municipality_Temperatura         `json:"temperatura"`
	SensTermica       Municipality_Temperatura         `json:"sensTermica"`
	HumedadRelativa   Municipality_Temperatura         `json:"humedadRelativa"`
	UvMax             int                             `json:"uvMax"`
	Fecha             string                          `json:"fecha"`
}

// Municipality_Prediccion represents the prediction structure
type Municipality_Prediccion struct {
	Dia []Municipality_Dia `json:"dia"`
}

// Municipality represents a municipality forecast
type Municipality struct {
	Elaborado  string                  `json:"elaborado"`
	Nombre     string                  `json:"nombre"`
	Provincia  string                  `json:"provincia"`
	Prediccion Municipality_Prediccion `json:"prediccion"`
	ID         int                     `json:"id"`
	Version    float64                 `json:"version"`
}
