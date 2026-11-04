package radar

import (
	"image"
	"image/color"
	"math"
	"time"
)

// pixelStatus is what the radar shows at a pixel.
type pixelStatus int

const (
	// dry means the pixel is covered by radar and shows no echo.
	dry pixelStatus = iota
	// echo means the pixel shows precipitation.
	echo
	// noCoverage means no radar sees the pixel.
	noCoverage
)

// colorTolerance is how far, per channel, a pixel may be from a legend color.
// AEMET's own map uses the same tolerance.
const colorTolerance = 10

// Image is a decoded composite with its georeference. The composite uses
// Web Mercator (EPSG:3857) between its bounds.
type Image struct {
	Time   time.Time
	img    image.Image
	bounds Bounds
	legend []Band
	w, h   int
	yNorth float64 // mercator y of the north edge
	ySouth float64 // mercator y of the south edge
}

func newImage(t time.Time, img image.Image, b Bounds, legend []Band) *Image {
	r := img.Bounds()
	return &Image{
		Time: t, img: img, bounds: b, legend: legend,
		w: r.Dx(), h: r.Dy(),
		yNorth: mercY(b.North), ySouth: mercY(b.South),
	}
}

func mercY(lat float64) float64 {
	return math.Log(math.Tan(math.Pi/4 + lat*math.Pi/360))
}

func invMercY(y float64) float64 {
	return (2*math.Atan(math.Exp(y)) - math.Pi/2) * 180 / math.Pi
}

// pixel returns the pixel containing lat/lon and whether it is in the image.
func (im *Image) pixel(lat, lon float64) (x, y int, ok bool) {
	b := im.bounds
	if lat <= b.South || lat >= b.North || lon <= b.West || lon >= b.East {
		return 0, 0, false
	}
	x = int((lon - b.West) / (b.East - b.West) * float64(im.w))
	y = int((im.yNorth - mercY(lat)) / (im.yNorth - im.ySouth) * float64(im.h))
	return x, y, x >= 0 && x < im.w && y >= 0 && y < im.h
}

// center returns the coordinates of the center of pixel x, y.
func (im *Image) center(x, y int) (lat, lon float64) {
	b := im.bounds
	lon = b.West + (float64(x)+0.5)/float64(im.w)*(b.East-b.West)
	lat = invMercY(im.yNorth - (float64(y)+0.5)/float64(im.h)*(im.yNorth-im.ySouth))
	return lat, lon
}

// classify returns the status of pixel x, y and its band when it is an echo.
func (im *Image) classify(x, y int) (pixelStatus, Band) {
	r := im.img.Bounds()
	c := color.NRGBAModel.Convert(im.img.At(r.Min.X+x, r.Min.Y+y)).(color.NRGBA)
	if c.A == 0 {
		return dry, Band{}
	}
	for _, b := range im.legend {
		if nearRGB(c, b.Color.R, b.Color.G, b.Color.B) {
			return echo, b
		}
	}
	// Some composites paint covered dry areas translucent black instead of
	// leaving them transparent.
	if nearRGB(c, 0, 0, 0) {
		return dry, Band{}
	}
	// Anything else, normally a light grey wash, marks areas no radar sees.
	return noCoverage, Band{}
}

func nearRGB(c color.NRGBA, r, g, b uint8) bool {
	return near(c.R, r) && near(c.G, g) && near(c.B, b)
}

func near(a, b uint8) bool {
	d := int(a) - int(b)
	return d >= -colorTolerance && d <= colorTolerance
}

// at returns what the radar shows at lat/lon. Points outside the image have
// no coverage.
func (im *Image) at(lat, lon float64) (pixelStatus, Band) {
	x, y, ok := im.pixel(lat, lon)
	if !ok {
		return noCoverage, Band{}
	}
	return im.classify(x, y)
}

const earthRadiusKm = 6371.0

// distanceKm is the great-circle distance between two points.
func distanceKm(lat1, lon1, lat2, lon2 float64) float64 {
	φ1, φ2 := lat1*math.Pi/180, lat2*math.Pi/180
	dφ, dλ := φ2-φ1, (lon2-lon1)*math.Pi/180
	a := math.Sin(dφ/2)*math.Sin(dφ/2) + math.Cos(φ1)*math.Cos(φ2)*math.Sin(dλ/2)*math.Sin(dλ/2)
	return 2 * earthRadiusKm * math.Asin(math.Min(1, math.Sqrt(a)))
}

var compass = []string{"N", "NE", "E", "SE", "S", "SW", "W", "NW"}

// bearing returns the 8-point compass direction from point 1 to point 2.
func bearing(lat1, lon1, lat2, lon2 float64) string {
	φ1, φ2 := lat1*math.Pi/180, lat2*math.Pi/180
	dλ := (lon2 - lon1) * math.Pi / 180
	θ := math.Atan2(math.Sin(dλ)*math.Cos(φ2), math.Cos(φ1)*math.Sin(φ2)-math.Sin(φ1)*math.Cos(φ2)*math.Cos(dλ))
	deg := math.Mod(θ*180/math.Pi+360, 360)
	return compass[int(math.Round(deg/45))%8]
}
