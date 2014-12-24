package main

import "fmt"

type metricSpec struct {
	metric_id string
	tags      map[string]string
}
type MetricEs struct {
	Tags []string `json:"tags"`
}

func NewMetricEs(spec metricSpec) MetricEs {
	tags := make([]string, len(spec.tags), len(spec.tags))
	i := 0
	for tag_key, tag_val := range spec.tags {
		tags[i] = fmt.Sprintf("%s=%s", tag_key, tag_val)
		i++
	}
	return MetricEs{tags}
}
