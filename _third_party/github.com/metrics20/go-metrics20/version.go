package metrics20

import (
	"strings"
)

type metricVersion int

const (
	legacy      metricVersion = iota // bar.bytes or whatever
	m20                              // foo=bar.unit=B
	m20NoEquals                      // foo_is_bar.unit_is_B
)

func getVersion(metric_in string) metricVersion {
	if strings.Contains(metric_in, "unit=") {
		return m20
	}
	if strings.Contains(metric_in, "unit_is_") {
		return m20NoEquals
	}
	return legacy
}

func is_metric20(metric_in string) bool {
	v := getVersion(metric_in)
	return v == m20 || v == m20NoEquals
}
