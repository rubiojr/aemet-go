package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/rubiojr/aemet-go"
	"github.com/urfave/cli/v3"
)

// formatDate converts date string from API format to a more readable format
func formatDate(dateStr string) string {
	// Parse the date (format: "2025-05-20T00:00:00")
	t, err := time.Parse("2006-01-02T15:04:05", dateStr)
	if err != nil {
		return dateStr // Return original if parsing fails
	}

	// Format as "Monday, Jan 02"
	return t.Format("Monday, Jan 02")
}

// getWindDirectionEmoji returns an emoji for the wind direction
func getWindDirectionEmoji(direction string) string {
	switch direction {
	case "N":
		return "⬇️" // North wind comes from north, blows south
	case "NE":
		return "↙️" // Northeast wind
	case "E":
		return "⬅️" // East wind
	case "SE":
		return "↖️" // Southeast wind
	case "S":
		return "⬆️" // South wind
	case "SW":
		return "↗️" // Southwest wind
	case "W":
		return "➡️" // West wind
	case "NW":
		return "↘️" // Northwest wind
	case "C":
		return "🔄" // Variable/calm
	default:
		return "💨" // Generic wind
	}
}

// getWeatherEmoji returns an emoji based on weather description and rain probability
func getWeatherEmoji(desc string, rainProb int) string {
	desc = strings.ToLower(desc)
	if rainProb > 70 {
		return "🌧️" // Rain
	} else if rainProb > 30 {
		return "🌦️" // Rain and sun
	} else if strings.Contains(desc, "tormenta") {
		return "⛈️" // Storm
	} else if strings.Contains(desc, "nieve") {
		return "❄️" // Snow
	} else if strings.Contains(desc, "niebla") {
		return "🌫️" // Fog
	} else if strings.Contains(desc, "nubos") {
		if strings.Contains(desc, "poco") {
			return "🌤️" // Partly cloudy
		} else if strings.Contains(desc, "muy") {
			return "☁️" // Very cloudy
		} else {
			return "⛅" // Cloudy
		}
	} else if strings.Contains(desc, "despejado") {
		return "☀️" // Sunny
	} else if strings.Contains(desc, "lluvia") {
		if strings.Contains(desc, "escasa") {
			return "🌦️" // Light rain
		}
		return "🌧️" // Rain
	} else {
		return "☀️" // Default sunny
	}
}

// PeriodData holds weather information for a specific time period
type PeriodData struct {
	RainProb  int
	SkyDesc   string
	WindDir   string
	WindSpeed int
}

// extractPeriodData extracts weather data for different time periods from a day forecast
func extractPeriodData(day aemet.Dia) (map[string]PeriodData, bool, bool, bool) {
	// Check if we have detailed periods or just 24h period
	hasMorningData := false
	hasAfternoonData := false
	has24hData := false

	// Map to store period data
	periodData := make(map[string]PeriodData)

	// Find data for each period
	for _, prob := range day.ProbPrecipitacion {
		// For last days which might not have a period field
		if prob.Periodo == "" {
			if pd, ok := periodData["default"]; ok {
				pd.RainProb = prob.Value
				periodData["default"] = pd
			} else {
				periodData["default"] = PeriodData{RainProb: prob.Value}
			}
			has24hData = true
		} else {
			if pd, ok := periodData[prob.Periodo]; ok {
				pd.RainProb = prob.Value
				periodData[prob.Periodo] = pd
			} else {
				periodData[prob.Periodo] = PeriodData{RainProb: prob.Value}
			}
			if prob.Periodo == "00-12" {
				hasMorningData = true
			} else if prob.Periodo == "12-24" {
				hasAfternoonData = true
			} else if prob.Periodo == "00-24" {
				has24hData = true
			}
		}
	}

	for _, sky := range day.EstadoCielo {
		// For last days which might not have a period field
		if sky.Periodo == "" {
			if pd, ok := periodData["default"]; ok {
				pd.SkyDesc = sky.Descripcion
				periodData["default"] = pd
			} else {
				periodData["default"] = PeriodData{SkyDesc: sky.Descripcion}
			}
			has24hData = true
		} else {
			if pd, ok := periodData[sky.Periodo]; ok {
				pd.SkyDesc = sky.Descripcion
				periodData[sky.Periodo] = pd
			} else {
				periodData[sky.Periodo] = PeriodData{SkyDesc: sky.Descripcion}
			}
			if sky.Periodo == "00-12" {
				hasMorningData = true
			} else if sky.Periodo == "12-24" {
				hasAfternoonData = true
			} else if sky.Periodo == "00-24" {
				has24hData = true
			}
		}
	}

	for _, wind := range day.Viento {
		// For last days which might not have a period field
		if wind.Periodo == "" {
			if pd, ok := periodData["default"]; ok {
				pd.WindDir = wind.Direccion
				pd.WindSpeed = wind.Velocidad
				periodData["default"] = pd
			} else {
				periodData["default"] = PeriodData{WindDir: wind.Direccion, WindSpeed: wind.Velocidad}
			}
			has24hData = true
		} else {
			if pd, ok := periodData[wind.Periodo]; ok {
				pd.WindDir = wind.Direccion
				pd.WindSpeed = wind.Velocidad
				periodData[wind.Periodo] = pd
			} else {
				periodData[wind.Periodo] = PeriodData{WindDir: wind.Direccion, WindSpeed: wind.Velocidad}
			}
			if wind.Periodo == "00-12" {
				hasMorningData = true
			} else if wind.Periodo == "12-24" {
				hasAfternoonData = true
			} else if wind.Periodo == "00-24" {
				has24hData = true
			}
		}
	}

	return periodData, hasMorningData, hasAfternoonData, has24hData
}

// displayPeriod prints weather information for a specific time period
func displayPeriod(periodName string, data PeriodData) {
	emoji := getWeatherEmoji(data.SkyDesc, data.RainProb)

	fmt.Printf("%s: %s ", periodName, emoji)
	if data.SkyDesc != "" {
		fmt.Printf("%s", data.SkyDesc)
	}
	if data.RainProb > 0 {
		fmt.Printf(" (💧 %d%%)", data.RainProb)
	}

	// Show wind information with directional emoji
	if data.WindDir != "" && data.WindSpeed > 0 {
		windEmoji := getWindDirectionEmoji(data.WindDir)
		fmt.Printf(" %s %s at %d km/h", windEmoji, data.WindDir, data.WindSpeed)
	}
	fmt.Println()
}

// get24hPeriodData combines data from 00-24 period and default (no period) data
func get24hPeriodData(periodData map[string]PeriodData) PeriodData {
	// Start with 00-24 data if available
	data := periodData["00-24"]

	// If default (no period) data exists, use it for missing values
	if defaultData, ok := periodData["default"]; ok {
		if data.RainProb == 0 && defaultData.RainProb > 0 {
			data.RainProb = defaultData.RainProb
		}
		if data.SkyDesc == "" && defaultData.SkyDesc != "" {
			data.SkyDesc = defaultData.SkyDesc
		}
		if data.WindDir == "" && defaultData.WindDir != "" {
			data.WindDir = defaultData.WindDir
			data.WindSpeed = defaultData.WindSpeed
		}
	}

	return data
}

// displayDayForecast displays the weather forecast for a single day
func displayDayForecast(day aemet.Dia) {
	// Format date nicely
	formattedDate := formatDate(day.Fecha)

	// Print date and temperature range
	fmt.Printf("\n📅 %s (🌡️ %d°C to %d°C)\n", formattedDate, day.Temperatura.Minima, day.Temperatura.Maxima)

	// Extract period data
	periodData, hasMorningData, hasAfternoonData, has24hData := extractPeriodData(day)

	// Display weather info based on available data
	if hasMorningData && hasAfternoonData {
		displayPeriod("Morning (00-12h)", periodData["00-12"])
		displayPeriod("Afternoon (12-24h)", periodData["12-24"])
	} else if has24hData {
		data := get24hPeriodData(periodData)
		displayPeriod("All day", data)
	}
}

// printForecastHeader displays the header for the weather forecast
func printForecastHeader(mun *aemet.Municipality) {
	currentTime := time.Now().Format("Monday, January 02 at 15:04")
	fmt.Printf("\n🌤️  Weather forecast for %s (%s)\n", mun.Nombre, mun.Provincia)
	fmt.Printf("📊 Forecast updated on %s\n", currentTime)
	fmt.Println("==============================================")
}

// displayForecast shows the weather forecast for the given municipality
func displayForecast(mun *aemet.Municipality) {
	// Print forecast header
	printForecastHeader(mun)

	// Display forecast for each day
	for _, day := range mun.Prediccion.Dia {
		displayDayForecast(day)
	}
}

// getDayForecastSummary returns a one-line weather summary for a municipality by name
func getDayForecastSummary(client *aemet.Client, municipalityName string) (string, error) {
	municipalities, err := aemet.FindMunicipalitiesByPartialName(municipalityName)
	if err != nil {
		return "", fmt.Errorf("error finding municipalities: %v", err)
	}

	if len(municipalities) == 0 {
		return "", fmt.Errorf("no municipalities found matching '%s'", municipalityName)
	}

	selectedMuni := municipalities[0]
	mun, err := client.GetForecastFor(selectedMuni.ID)
	if err != nil {
		return "", fmt.Errorf("error getting weather data: %v", err)
	}

	return buildWeatherSummary(mun)
}

// getDayForecastSummaryByID returns a one-line weather summary for a municipality by ID
func getDayForecastSummaryByID(client *aemet.Client, municipalityID string) (string, error) {
	mun, err := client.GetForecastFor(municipalityID)
	if err != nil {
		return "", fmt.Errorf("error getting weather data: %v", err)
	}

	return buildWeatherSummary(mun)
}

// buildWeatherSummary creates a weather summary string from municipality data
func buildWeatherSummary(mun *aemet.Municipality) (string, error) {
	if len(mun.Prediccion.Dia) == 0 {
		return "", fmt.Errorf("no forecast data available")
	}

	today := mun.Prediccion.Dia[0]
	periodData, hasMorningData, hasAfternoonData, has24hData := extractPeriodData(today)

	var skyDesc string
	var rainProb int
	var windDir string
	var windSpeed int

	if hasMorningData && hasAfternoonData {
		morningData := periodData["00-12"]
		afternoonData := periodData["12-24"]
		if morningData.RainProb > afternoonData.RainProb {
			rainProb = morningData.RainProb
			skyDesc = morningData.SkyDesc
		} else {
			rainProb = afternoonData.RainProb
			skyDesc = afternoonData.SkyDesc
		}
		if morningData.WindSpeed > afternoonData.WindSpeed {
			windDir = morningData.WindDir
			windSpeed = morningData.WindSpeed
		} else {
			windDir = afternoonData.WindDir
			windSpeed = afternoonData.WindSpeed
		}
	} else if has24hData {
		data := get24hPeriodData(periodData)
		skyDesc = data.SkyDesc
		rainProb = data.RainProb
		windDir = data.WindDir
		windSpeed = data.WindSpeed
	}

	emoji := getWeatherEmoji(skyDesc, rainProb)
	summary := fmt.Sprintf("%s %s: %s %d°C-%d°C", emoji, mun.Nombre, skyDesc, today.Temperatura.Minima, today.Temperatura.Maxima)

	if rainProb > 0 {
		summary += fmt.Sprintf(" (💧 %d%%)", rainProb)
	}

	if windDir != "" && windSpeed > 0 {
		windEmoji := getWindDirectionEmoji(windDir)
		summary += fmt.Sprintf(" %s %d km/h", windEmoji, windSpeed)
	}

	return summary, nil
}

// dayCommand handles the day subcommand
func dayCommand(ctx context.Context, cmd *cli.Command) error {
	cities := cmd.StringSlice("cities")
	if len(cities) == 0 {
		return fmt.Errorf("at least one city name is required")
	}

	useIDs := cmd.Bool("use-ids")

	client, err := aemet.NewWithDefaults()
	if err != nil {
		return fmt.Errorf("error creating client: %v", err)
	}

	fmt.Printf("🌤️  El tiempo hoy\n")
	fmt.Println("==============================================")

	for _, city := range cities {
		var summary string
		var err error

		if useIDs {
			summary, err = getDayForecastSummaryByID(client, city)
		} else {
			summary, err = getDayForecastSummary(client, city)
		}

		if err != nil {
			fmt.Printf("❌ %s: %v\n", city, err)
			continue
		}
		fmt.Println(summary)
	}

	return nil
}

// forecastCommand handles the forecast subcommand
func forecastCommand(ctx context.Context, cmd *cli.Command) error {
	// Get the municipality name from args
	partialName := cmd.String("name")
	if partialName == "" {
		return fmt.Errorf("municipality name is required")
	}

	// Create the AEMET client
	client, err := aemet.NewWithDefaults()
	if err != nil {
		return fmt.Errorf("error creating client: %v", err)
	}

	// Find municipalities by partial name
	municipalities, err := aemet.FindMunicipalitiesByPartialName(partialName)
	if err != nil {
		return fmt.Errorf("error finding municipalities: %v", err)
	}

	if len(municipalities) == 0 {
		return fmt.Errorf("no municipalities found matching '%s'", partialName)
	}

	// If multiple matches found and interactive mode not disabled, ask user to select
	selectedMuni := municipalities[0]
	if len(municipalities) > 1 && !cmd.Bool("non-interactive") {
		fmt.Printf("Found %d municipalities matching '%s':\n\n", len(municipalities), partialName)

		for i, muni := range municipalities {
			fmt.Printf("%d. %s (%s)\n", i+1, muni.Name, muni.Capital)
		}

		fmt.Print("\nSelect a municipality (1-" + fmt.Sprintf("%d", len(municipalities)) + "): ")
		var selection int
		fmt.Scanln(&selection)

		if selection < 1 || selection > len(municipalities) {
			return fmt.Errorf("invalid selection")
		}

		selectedMuni = municipalities[selection-1]
	} else if len(municipalities) > 1 {
		fmt.Printf("Found %d municipalities matching '%s', using first match: %s\n",
			len(municipalities), partialName, selectedMuni.Name)
	}

	// Get the weather forecast
	mun, err := client.GetForecastFor(selectedMuni.ID)
	if err != nil {
		return fmt.Errorf("error getting weather data: %v", err)
	}

	// Display the forecast
	displayForecast(mun)
	return nil
}

func main() {
	// Create the CLI application
	app := &cli.Command{
		Name:  "aemet",
		Usage: "AEMET weather data CLI tool",
		Commands: []*cli.Command{
			{
				Name:    "forecast",
				Aliases: []string{"f"},
				Usage:   "Get weather forecast for a municipality",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "name",
						Aliases:  []string{"n"},
						Usage:    "Municipality name (partial match)",
						Required: true,
					},
					&cli.BoolFlag{
						Name:    "non-interactive",
						Aliases: []string{"i"},
						Usage:   "Non-interactive mode (automatically selects first match)",
					},
				},
				Action: forecastCommand,
			},
			{
				Name:    "day",
				Aliases: []string{"d"},
				Usage:   "Get today's weather summary for multiple cities",
				Flags: []cli.Flag{
					&cli.StringSliceFlag{
						Name:     "cities",
						Aliases:  []string{"c"},
						Usage:    "List of city names (partial match allowed)",
						Required: true,
					},
					&cli.BoolFlag{
						Name:    "use-ids",
						Aliases: []string{"ids"},
						Usage:   "Treat cities as municipality IDs instead of names",
					},
				},
				Action: dayCommand,
			},
		},
	}

	// Run the application
	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
