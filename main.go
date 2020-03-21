package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
)

var logger = log.New(os.Stdout, "covid19-at", 0)
var mp = newMetadataProvider()

var hmExporter = newHealthMinistryExporter()
var smExporter = newSocialMinistryExporter(mp)
var ecdcExp = newEcdcExporter(mp)

var exporters = []Exporter{
	hmExporter,
	smExporter,
	ecdcExp,
}

type summaryReport struct {
	Overview struct {
		Hospitalized  uint64
		IntensiveCare uint64
		Tested        uint64
		Infected      uint64
		Healed        uint64
		Dead          uint64
	}
	Precincts []CovidStat
	States    []CovidStat
	Countries []CovidStat
}

func handleMetrics(w http.ResponseWriter, _ *http.Request) {
	for _, e := range exporters {
		metrics, err := e.GetMetrics()
		if err == nil {
			writeMetrics(metrics, w)
		}
	}
}

func handleMetricsJson(w http.ResponseWriter, r *http.Request) {
	//r.Form.Get()
	w.Header().Add("Content-Type", "application/json; charset=utf-8")

	data := summaryReport{}

	smMetrics, err := smExporter.GetMetrics()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		data.Overview.Dead = uint64(smMetrics.findMetric("cov19_dead", "").Value)
		data.Overview.Tested = uint64(smMetrics.findMetric("cov19_tests", "").Value)
		data.Overview.Healed = uint64(smMetrics.findMetric("cov19_healed", "").Value)
	}

	hmMetrics, errs := hmExporter.getSimpleData()
	if len(errs) != 0 {
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		data.Overview.Hospitalized = uint64(hmMetrics.findMetric("cov19_hospitalized", "").Value)
		data.Overview.IntensiveCare = uint64(hmMetrics.findMetric("cov19_intensive_care", "").Value)
		data.Overview.Infected = uint64(hmMetrics.findMetric("cov19_confirmed", "").Value)
	}

	ecdcStats, err := ecdcExp.getEcdcCovidStat()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		data.Countries = ecdcStats
	}

	response, err := json.Marshal(data)
	if err != nil {
		panic(err)
		w.WriteHeader(http.StatusInternalServerError)

	} else {
		w.Write(response)
	}

}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	errors := make([]error, 0)
	for _, e := range exporters {
		errors = append(errors, e.Health()...)
	}

	if len(errors) > 0 {
		w.WriteHeader(http.StatusInternalServerError)
		errorResponse := ""
		for _, e := range errors {
			errorResponse += e.Error() + "\n"
		}
		fmt.Fprintf(w, `<html><body><img width="500" src="https://spiessknafl.at/fine.jpg"/><pre>%s</pre></body></html>`, errorResponse)
	} else {
		fmt.Fprintf(w, `<html><body><img width="500" src="https://spiessknafl.at/helth.png"/></body></html>`)
	}
}

func main() {
	http.HandleFunc("/metrics", handleMetrics)
	http.HandleFunc("/metrics/current", handleMetricsJson)
	http.HandleFunc("/health", handleHealth)
	http.ListenAndServe(":8282", nil)
}
