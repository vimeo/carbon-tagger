package main

import (
	"errors"
	"fmt"
	"strings"
)

type metricSpec struct {
	metric_id string
	tags      map[string]string
}

func NewMetricSpec(metric_line string) (metric *metricSpec, err error) {
	// metric_spec value unix_timestamp
	elements := strings.Split(metric_line, " ")
	metric_id := ""
	if len(elements) != 3 {
		return nil, errors.New(fmt.Sprintf("metric doesn't contain exactly 3 nodes: %s", metric_line))
	}
	metric_id = elements[0]
	nodes := strings.Split(metric_id, ".")
	tags := make(map[string]string)
	for i, node := range nodes {
		var tag []string
		if strings.Contains(node, "_is_") {
			tag = strings.Split(node, "_is_")
		} else {
			tag = strings.Split(node, "=")
		}
		if len(tag) > 2 {
			return nil, errors.New("bad metric spec: more than 1 equals")
		} else if len(tag) < 2 {
			tags[fmt.Sprintf("n%d", i+1)] = node
		} else if tag[0] == "" || tag[1] == "" {
			return nil, errors.New("bad metric spec: tag_k and tag_v must be non-empty strings")
		} else {
			// k=v format, and both are != ""
			tags[tag[0]] = tag[1]
		}
	}
	if u, ok := tags["unit"]; !ok {
		return nil, errors.New("bad metric spec: unit tag (mandatory) not specified")
	} else if strings.HasSuffix(u, "ps") {
		tags["unit"] = u[:len(u)-2] + "/s"
	}

	if len(tags) < 2 {
		return nil, errors.New("bad metric spec: must have at least one tag_k/tag_v pair beyond unit")
	}
	return &metricSpec{metric_id, tags}, nil
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
