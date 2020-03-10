package main

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"

	"github.com/PuerkitoBio/goquery"
)

//TotalStat for Cov19 in Austria
type TotalStat struct {
	tests     int
	confirmed int
	healed    int
}

//ProvinceStat for Cov19
type ProvinceStat struct {
	name  string
	count int
}

//WorldStat for Cov19 infections and deaths
type WorldStat struct {
	continent string
	country   string
	infected  int
	deaths    int
}

func parseStats(reader io.Reader) TotalStat {
	document, err := goquery.NewDocumentFromReader(reader)
	if err != nil {
		return TotalStat{0, 0, 0}
	}

	summary, err := document.Find(".abstract").First().Html()

	confirmedMatch := regexp.MustCompile(`Fälle: [^0-9]*([0-9]+)`).FindStringSubmatch(summary)
	healed := 0
	confirmed := 0
	tests := 0
	if len(confirmedMatch) >= 2 {
		confirmed = atoi(confirmedMatch[1])
	}

	testsMatch := regexp.MustCompile(`Testungen: [^0-9]*(?P<number>[0-9]+)`).FindAllStringSubmatch(summary, -1)
	if len(testsMatch) >= 1 && len(testsMatch[0]) >= 2 {
		tests = atoi(testsMatch[0][1])
	}

	healedMatch := regexp.MustCompile(`Genesene Personen: [^0-9]*([0-9]+)`).FindStringSubmatch(summary)
	if len(healedMatch) >= 2 {
		healed = atoi(healedMatch[1])
	}
	return TotalStat{tests: tests, confirmed: confirmed, healed: healed}
}

func getStats() TotalStat {
	response, err := http.Get("https://www.sozialministerium.at/Informationen-zum-Coronavirus/Neuartiges-Coronavirus-(2019-nCov).html")
	if err != nil {
		return TotalStat{0, 0, 0}
	}
	defer response.Body.Close()

	return parseStats(response.Body)

}

func parseProvinceStats(r io.Reader) []ProvinceStat {
	document, _ := goquery.NewDocumentFromReader(r)
	summary, _ := document.Find(".infobox").Html()
	re := regexp.MustCompile(`(?P<location>\S+) \((?P<number>\d+)\)`)
	matches := re.FindAllStringSubmatch(summary, -1)

	result := make([]ProvinceStat, len(matches))
	for i, match := range matches {
		number := atoi(match[2])
		result[i] = ProvinceStat{match[1], number}
	}

	return result
}

func getDetails() []ProvinceStat {
	response, err := http.Get("https://www.sozialministerium.at/Informationen-zum-Coronavirus/Neuartiges-Coronavirus-(2019-nCov).html")
	if err != nil {
		fmt.Println("Error get request")
		return []ProvinceStat{}
	}
	defer response.Body.Close()

	return parseProvinceStats(response.Body)
}

func getWorldStats() []WorldStat {
	response, err := http.Get("https://www.ecdc.europa.eu/en/geographical-distribution-2019-ncov-cases")
	if err != nil {
		fmt.Println("Error get request")

	}
	defer response.Body.Close()

	document, err := goquery.NewDocumentFromReader(response.Body)
	table := document.Find("table").Find("tbody")
	if table == nil {
		fmt.Println("Error getting world stats")
	}

	rows := table.Find("tr")
	result := make([]WorldStat, rows.Size()-1)

	rows.Each(func(i int, s *goquery.Selection) {
		if i < rows.Size()-1 {
			rowStart := s.Find("td").First()
			result[i] = WorldStat{
				continent: rowStart.Text(),
				country:   rowStart.Next().Text(),
				infected:  atoi(rowStart.Next().Next().Text()),
				deaths:    atoi(rowStart.Next().Next().Next().Text()),
			}
		}
	})
	return result
}

func atoi(s string) int {
	result, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return result
}

var provinces = [9]string{"Wien", "Niederösterreich", "Oberösterreich", "Salzburg", "Tirol", "Vorarlberg", "Steiermark", "Burgenland", "Kärnten"}

func isAustria(location string) bool {
	for _, province := range provinces {
		if province == location {
			return true
		}
	}
	return false
}

func handleMetrics(w http.ResponseWriter, r *http.Request) {
	stats := getStats()
	fmt.Fprintf(w, "cov19_tests %d\n", stats.tests)
	fmt.Fprintf(w, "cov19_confirmed %d\n", stats.confirmed)
	fmt.Fprintf(w, "cov19_healed %d\n", stats.healed)

	details := getDetails()
	fmt.Println("Details: ", details)
	for _, detail := range details {
		if isAustria(detail.name) {
			fmt.Fprintf(w, "cov19_detail{country=\"Austria\",province=\"%s\"} %d\n", detail.name, detail.count)
		} else {
			fmt.Fprintf(w, "cov19_detail{country=\"%s\"} %d\n", detail.name, detail.count)
		}
	}

	for _, s := range getWorldStats() {
		fmt.Fprintf(w, "cov19_world_death{continent=\"%s\",country=\"%s\"} %d\n", s.continent, s.country, s.deaths)
		fmt.Fprintf(w, "cov19_world_infected{continent=\"%s\",country=\"%s\"} %d\n", s.continent, s.country, s.infected)
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	summary := getStats()
	details := getDetails()
	world := getWorldStats()

	failures := 0

	if summary.confirmed == 0 {
		failures++
		fmt.Fprintf(w, "Summary confirmed are failing\n")
	}

	if summary.healed == 0 {
		failures++
		fmt.Fprintf(w, "Summary healed are failing\n")
	}

	if summary.tests == 0 {
		failures++
		fmt.Fprintf(w, "Summary tests are failing\n")
	}

	if len(details) == 0 || details[0].count == 0 {
		failures++
		fmt.Fprintf(w, "Details Austria are failing\n")
	}

	if len(world) == 0 {
		failures++
		fmt.Fprintf(w, "World stats are failing\n")
	}

	if failures > 0 {
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		fmt.Fprintf(w, "Everything is fine :)\n")
	}
}

func main() {
	http.HandleFunc("/metrics", handleMetrics)
	http.HandleFunc("/health", handleHealth)
	http.ListenAndServe(":8282", nil)
}
