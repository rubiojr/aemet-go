package aemet

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed data/municipalities.json
var municipalitiesFS embed.FS

// Municipality represents a municipality in the AEMET API
type MunicipalityInfo struct {
	Latitude     string `json:"latitud"`
	IDOld        string `json:"id_old"`
	URL          string `json:"url"`
	LatitudeDec  string `json:"latitud_dec"`
	Altitude     string `json:"altitud"`
	Capital      string `json:"capital"`
	NumHab       string `json:"num_hab"`
	ZonaComarcal string `json:"zona_comarcal"`
	Destacada    string `json:"destacada"`
	Name         string `json:"nombre"`
	LongitudeDec string `json:"longitud_dec"`
	ID           string `json:"id"`
	Longitude    string `json:"longitud"`
}

var municipalities []*MunicipalityInfo
var initialized bool

// Initialize loads the municipality data from the embedded file
func initializeMunicipalities() error {
	if initialized {
		return nil
	}

	data, err := municipalitiesFS.ReadFile("data/municipalities.json")
	if err != nil {
		return fmt.Errorf("error reading municipalities data: %w", err)
	}

	err = json.Unmarshal(data, &municipalities)
	if err != nil {
		return fmt.Errorf("error parsing municipalities data: %w", err)
	}

	for _, m := range municipalities {
		m.ID = strings.TrimLeft(m.ID, "id")
	}

	initialized = true
	return nil
}

// FindMunicipalityID searches for a municipality by name and returns its ID
func FindMunicipalityID(name string) (string, error) {
	if err := initializeMunicipalities(); err != nil {
		return "", err
	}

	normalizedName := strings.ToLower(strings.TrimSpace(name))

	for _, muni := range municipalities {
		if strings.ToLower(muni.Name) == normalizedName {
			// Strip "id" prefix if present
			if strings.HasPrefix(muni.ID, "id") {
				return muni.ID[2:], nil
			}
			return muni.ID, nil
		}
	}

	return "", fmt.Errorf("municipality not found: %s", name)
}

// FindMunicipalitiesByPartialName searches for municipalities by partial name match
func FindMunicipalitiesByPartialName(partialName string) ([]*MunicipalityInfo, error) {
	if err := initializeMunicipalities(); err != nil {
		return nil, err
	}

	normalizedPartial := strings.ToLower(strings.TrimSpace(partialName))
	var results []*MunicipalityInfo

	for _, muni := range municipalities {
		if strings.Contains(strings.ToLower(muni.Name), normalizedPartial) {
			results = append(results, muni)
		}
	}

	return results, nil
}

// GetAllMunicipalities returns all municipalities
func GetAllMunicipalities() ([]*MunicipalityInfo, error) {
	if err := initializeMunicipalities(); err != nil {
		return nil, err
	}
	return municipalities, nil
}

// GetMunicipalityByID returns a municipality by its ID
func GetMunicipalityByID(id string) (*MunicipalityInfo, error) {
	if err := initializeMunicipalities(); err != nil {
		return nil, err
	}

	// Add "id" prefix if not present
	searchID := id
	if !strings.HasPrefix(searchID, "id") {
		searchID = "id" + searchID
	}

	for _, muni := range municipalities {
		if muni.ID == searchID {
			return muni, nil
		}
	}

	return nil, fmt.Errorf("municipality ID not found: %s", id)
}
