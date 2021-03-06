package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"time"
)

type healthMinistryExporter struct {
	mp  *metadataProvider
	url string
}

type ministryStat []struct {
	Label string
	Y     uint64
}

func newHealthMinistryExporter() *healthMinistryExporter {
	return &healthMinistryExporter{mp: newMetadataProviderWithFilename("bezirke.csv"), url: "https://info.gesundheitsministerium.at/data"}
}

func checkTags(result metrics, field string) []error {
	errors := make([]error, 0)
	for _, s := range result {
		if len(*s.Tags) != 4 {
			errors = append(errors, fmt.Errorf("Missing tags for: %s", (*s.Tags)[field]))
		}
	}
	return errors
}

func (h *healthMinistryExporter) GetMetrics() (metrics, error) {
	metrics := make(metrics, 0)

	result, _ := h.getSimpleData()
	metrics = append(metrics, result...)

	result, err := h.getAgeMetrics()
	metrics = append(metrics, result...)

	result, err = h.getGeschlechtsVerteilung()
	metrics = append(metrics, result...)

	result, err = h.getBundeslandInfectedMetric()
	metrics = append(metrics, result...)

	result, err = h.getBezirkMetric()
	metrics = append(metrics, result...)

	return metrics, err
}

func (h *healthMinistryExporter) Health() []error {
	errors := make([]error, 0)
	result, err := h.getBezirkMetric()
	if err != nil {
		errors = append(errors, err)
	}
	if len(result) < 10 {
		errors = append(errors, fmt.Errorf("Not enough Bezirke Results: %d", len(result)))
	}
	errors = append(errors, checkTags(result, "bezirk")...)

	result, err = h.getBundeslandInfectedMetric()
	if err != nil {
		errors = append(errors, err)
	}
	if len(result) != 27 {
		errors = append(errors, fmt.Errorf("Missing Bundesland result %d", len(result)))
	}
	errors = append(errors, checkTags(result, "province")...)

	result, err = h.getAgeMetrics()
	if err != nil {
		errors = append(errors, err)
	}
	if len(result) < 4 {
		errors = append(errors, fmt.Errorf("Missing age metrics"))
	}

	result, err = h.getGeschlechtsVerteilung()
	if err != nil {
		errors = append(errors, err)
	}
	if len(result) != 2 {
		errors = append(errors, fmt.Errorf("Geschlechtsverteilung failed"))
	}

	result, err2 := h.getSimpleData()
	errors = append(errors, err2...)

	if len(result) < 1 {
		errors = append(errors, fmt.Errorf("Could not find \"Bestätigte Fälle\""))
	}

	return errors
}

func (h *healthMinistryExporter) getTags(location string, fieldName string, data *metaData) *map[string]string {
	if data != nil {
		return &map[string]string{fieldName: location, "country": "Austria", "longitude": ftos(data.location.long), "latitude": ftos(data.location.lat)}
	}
	return &map[string]string{fieldName: location, "country": "Austria"}
}

func (h *healthMinistryExporter) getBezirkStat() ([]bezirkStat, error) {
	arrayString, err := readArrayFromGet(h.url + "/Bezirke.js")
	if err != nil {
		return nil, err
	}
	bezirkeStats := ministryStat{}
	err = json.Unmarshal([]byte(arrayString), &bezirkeStats)
	if err != nil {
		return nil, err
	}
	result := make([]bezirkStat, 0)
	for _, s := range bezirkeStats {
		data := h.mp.getMetadata(s.Label)
		result = append(result, bezirkStat{s.Label, apiLocaiton{Lat: data.location.lat, Long: data.location.long}, data.population, s.Y})
	}
	return result, nil
}

func (h *healthMinistryExporter) getBezirkMetric() (metrics, error) {
	stats, err := h.getBezirkStat()
	if err != nil {
		return nil, err
	}
	result := make(metrics, 0)
	for _, s := range stats {
		data := h.mp.getMetadata(s.Name)
		tags := h.getTags(s.Name, "bezirk", data)
		result = append(result, metric{"cov19_bezirk_infected", tags, float64(s.Infected)})
		if data != nil {
			result = append(result, metric{"cov19_bezirk_infected_100k", tags, float64(infection100k(s.Infected, data.population))})
		}
	}
	return result, nil
}

func mapBundeslandLabel(label string) string {
	switch label {
	case "Ktn":
		return "Kärnten"
	case "NÖ":
		return "Niederösterreich"
	case "OÖ":
		return "Oberösterreich"
	case "Sbg":
		return "Salzburg"
	case "Stmk":
		return "Steiermark"
	case "T":
		return "Tirol"
	case "Vbg":
		return "Vorarlberg"
	case "W":
		return "Wien"
	case "Bgld":
		return "Burgenland"
	}
	return "unknown"
}

func (h *healthMinistryExporter) getBundeslandInfectedMetric() (metrics, error) {
	bundeslandStat, err := h.getBundeslandInfected()
	if err != nil {
		return nil, err
	}
	result := make(metrics, 0)
	for k, v := range bundeslandStat {
		data := h.mp.getMetadata(k)
		tags := h.getTags(k, "province", data)
		result = append(result, metric{"cov19_detail", tags, float64(v)})
		if data != nil {
			result = append(result, metric{"cov19_detail_infected_per_100k", tags, float64(infection100k(v, data.population))})
			result = append(result, metric{"cov19_detail_infection_rate", tags, float64(infectionRate(v, data.population))})
		}
	}
	return result, nil
}

func (h *healthMinistryExporter) getBundeslandInfected() (map[string]uint64, error) {
	arrayString, err := readArrayFromGet(h.url + "/Bundesland.js")
	if err != nil {
		return nil, err
	}
	bundeslandStats := ministryStat{}
	err = json.Unmarshal([]byte(arrayString), &bundeslandStats)
	if err != nil {
		return nil, err
	}
	result := make(map[string]uint64)
	for _, s := range bundeslandStats {
		s.Label = mapBundeslandLabel(s.Label)
		result[s.Label] = s.Y
	}
	return result, nil
}

func (h *healthMinistryExporter) getAgeStat() (map[string]uint64, error) {
	arrayString, err := readArrayFromGet(h.url + "/Altersverteilung.js")
	if err != nil {
		return nil, err
	}
	ageStats := ministryStat{}
	err = json.Unmarshal([]byte(arrayString), &ageStats)
	if err != nil {
		return nil, err
	}
	result := make(map[string]uint64)
	for _, s := range ageStats {
		result[s.Label] = s.Y
	}
	return result, nil
}

func (h *healthMinistryExporter) getAgeMetrics() (metrics, error) {
	ageMetrics, err := h.getAgeStat()
	if err != nil {
		return nil, err
	}
	result := make(metrics, 0)
	for k, v := range ageMetrics {
		tags := &map[string]string{"country": "Austria", "group": k}
		result = append(result, metric{"cov19_age_distribution", tags, float64(v)})
	}
	return result, nil
}

func (h *healthMinistryExporter) getGeschlechtsVerteilung() (metrics, error) {
	arrayString, err := readArrayFromGet(h.url + "/Geschlechtsverteilung.js")
	if err != nil {
		return nil, err
	}
	ageStats := ministryStat{}
	err = json.Unmarshal([]byte(arrayString), &ageStats)
	if err != nil {
		return nil, err
	}
	result := make(metrics, 0)
	for _, s := range ageStats {
		tags := &map[string]string{"country": "Austria", "sex": s.Label}
		result = append(result, metric{"cov19_sex_distribution", tags, float64(s.Y)})
	}
	return result, nil
}

func (h *healthMinistryExporter) getSimpleData() (metrics, []error) {
	client := http.Client{Timeout: 5 * time.Second}
	errors := make([]error, 0)
	response, err := client.Get(h.url + "/SimpleData.js")
	result := make(metrics, 0)
	if err != nil {
		return nil, []error{err}
	}
	defer response.Body.Close()
	lines, err := ioutil.ReadAll(response.Body)

	erkrankungenMatch := regexp.MustCompile(`Erkrankungen = ([0-9]+)`).FindStringSubmatch(string(lines))
	if len(erkrankungenMatch) != 2 {
		errors = append(errors, fmt.Errorf("Could not find \"Bestätigte Fälle\""))
	} else {
		result = append(result, metric{"cov19_confirmed", nil, atof(erkrankungenMatch[1])})
	}
	return result, errors
}
