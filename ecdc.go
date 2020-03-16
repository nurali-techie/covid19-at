package main

import 	(
	"github.com/PuerkitoBio/goquery"
	"net/http"
	"errors"
)

//EcdcExporter for parsing tables
type EcdcExporter struct {
	url string
}

//EcdcStat for Cov19 infections and deaths
type EcdcStat struct {
	continent string
	country   string
	infected  uint64
	deaths    uint64
}

//NewEcdcExporter creates a new exporter
func NewEcdcExporter() *EcdcExporter {
	return &EcdcExporter{url: "https://www.ecdc.europa.eu/en/geographical-distribution-2019-ncov-cases"}
}

//GetMetrics parses the ECDC table
func (e *EcdcExporter) GetMetrics() ([]Metric, error) {
	stats, err := getEcdcStat(e.url)
	if err != nil {
		return nil, err
	}
	result := make([]Metric, 2*len(stats))
	for i := range stats {
		tags := map[string]string{"country": stats[i].country, "continent": stats[i].continent}
		result[2*i].Tags = &tags
		result[2*i].Name = "cov19_world_death"
		result[2*i].Value = stats[i].deaths
		result[2*i+1].Tags = &tags
		result[2*i+1].Name = "cov19_world_infected"
		result[2*i+1].Value = stats[i].infected
	}
	return result, nil
}

func getEcdcStat(url string) ([]EcdcStat,error) {
	response, err := http.Get(url)
	if err != nil  {
		return nil, err
	}
	defer response.Body.Close()

	document, _ := goquery.NewDocumentFromReader(response.Body)
	rows := document.Find("table").Find("tbody").Find("tr")
	if rows.Size() == 0 {
		return nil, errors.New("Could not find table")
	}

	result := make([]EcdcStat, rows.Size()-1)

	rows.Each(func(i int, s *goquery.Selection) {
		if i < rows.Size()-1 {
			rowStart := s.Find("td").First()
			result[i] = EcdcStat{
				continent: rowStart.Text(),
				country:   rowStart.Next().Text(),
				infected:  atoi(rowStart.Next().Next().Text()),
				deaths:    atoi(rowStart.Next().Next().Next().Text()),
			}
		}
	})
	return result,nil
}