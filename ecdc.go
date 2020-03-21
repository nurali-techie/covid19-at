package main

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/PuerkitoBio/goquery"
)

type ecdcExporter struct {
	Url string
	Mp  *metadataProvider
}

type ecdcStat struct {
	CovidStat
	continent string
}

func newEcdcExporter(lp *metadataProvider) *ecdcExporter {
	return &ecdcExporter{Url: "https://www.ecdc.europa.eu/en/geographical-distribution-2019-ncov-cases", Mp: lp}
}

//GetMetrics parses the ECDC table
func (e *ecdcExporter) GetMetrics() (metrics, error) {
	stats, err := e.getEcdcStat()
	if err != nil {
		return nil, err
	}
	result := make([]metric, 0)
	for i := range stats {
		tags := e.getTags(stats, i)
		deaths := stats[i].Deaths
		infected := stats[i].Infected
		population := e.Mp.getPopulation(stats[i].Location)
		if deaths > 0 {
			result = append(result, metric{Name: "cov19_world_death", Value: float64(deaths), Tags: &tags})
			if population > 0 {
				result = append(result, metric{Name: "cov19_world_fatality_rate", Value: fatalityRate(infected, deaths), Tags: &tags})
			}
		}
		result = append(result, metric{Name: "cov19_world_infected", Value: float64(infected), Tags: &tags})
		if population > 0 {
			result = append(result, metric{Name: "cov19_world_infection_rate", Value: infectionRate(infected, population), Tags: &tags})
			result = append(result, metric{Name: "cov19_world_infected_per_100k", Value: infection100k(infected, population), Tags: &tags})
		}
	}
	return result, nil
}

//Health checks the functionality of the exporter
func (e *ecdcExporter) Health() []error {
	errors := make([]error, 0)
	worldStats, _ := e.GetMetrics()

	if len(worldStats) < 200 {
		errors = append(errors, fmt.Errorf("World stats are failing"))
	}

	for _, m := range worldStats {
		country := (*m.Tags)["country"]
		if mp.getLocation(country) == nil {
			errors = append(errors, fmt.Errorf("Could not find Location for country: %s", country))
		}
	}
	return errors
}

func normalizeCountryName(name string) string {
	name = strings.TrimSpace(name)
	parts := strings.FieldsFunc(name, func(r rune) bool { return r == ' ' || r == '_' })
	for i, part := range parts {
		if strings.ToUpper(part) == "AND" || strings.ToUpper(part) == "OF" {
			parts[i] = strings.ToLower(part)
		} else {
			runes := []rune(part)
			parts[i] = string(unicode.ToUpper(runes[0])) + strings.ToLower(string(runes[1:]))
		}
	}

	return strings.Join(parts, " ")
}

func (e *ecdcExporter) getTags(stats []ecdcStat, i int) map[string]string {
	var tags map[string]string
	if e.Mp != nil && e.Mp.getLocation(stats[i].Location) != nil {
		location := e.Mp.getLocation(stats[i].Location)
		tags = map[string]string{"country": stats[i].Location, "continent": stats[i].continent, "latitude": ftos(location.Lat), "longitude": ftos(location.Long)}
	} else {
		tags = map[string]string{"country": stats[i].Location, "continent": stats[i].continent}
	}
	return tags
}

func (e *ecdcExporter) getEcdcCovidStat() ([]CovidStat, error) {
	stats, err := e.getEcdcStat()
	if err != nil {
		return nil, err
	}
	result := make([]CovidStat, len(stats))
	for i, s := range stats {
		result[i].Location = s.Location
		result[i].Infected = s.Infected
		result[i].Deaths = s.Deaths
		result[i].Geo = s.Geo
		result[i].Population = s.Population
	}
	return result, nil
}

func (e *ecdcExporter) getEcdcStat() ([]ecdcStat, error) {
	client := http.Client{Timeout: 3 * time.Second}
	response, err := client.Get(e.Url)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	document, _ := goquery.NewDocumentFromReader(response.Body)
	rows := document.Find("table").Find("tbody").Find("tr")
	if rows.Size() == 0 {
		return nil, errors.New("Could not find table")
	}

	result := make([]ecdcStat, rows.Size()-1)

	rows.Each(func(i int, s *goquery.Selection) {
		if i < rows.Size()-1 {
			rowStart := s.Find("td").First()

			location := normalizeCountryName(rowStart.Next().Text())
			metaData := e.Mp.getMetadata(location)
			if metaData != nil {
				result[i] = ecdcStat{
					CovidStat{
						Location:   location,
						Infected:   atoi(rowStart.Next().Next().Text()),
						Deaths:     atoi(rowStart.Next().Next().Next().Text()),
						Population: (*metaData).population,
						Geo:        (*metaData).location,
					},
					rowStart.Text(),
				}
			} else {
				result[i] = ecdcStat{
					CovidStat{
						Location: location,
						Infected: atoi(rowStart.Next().Next().Text()),
						Deaths:   atoi(rowStart.Next().Next().Next().Text()),
					},
					rowStart.Text(),
				}
			}
		}
	})
	return result, nil
}
