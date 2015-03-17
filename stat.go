package main

import (
	"fmt"
	"github.com/Dieterbe/go-metrics"
)

// note in metrics2.0 counter is a type of gauge that only increases
// in go-metrics:
// - a counter can also decrease (!), and can't be set to a value (though you can clear + inc(val))
// - a gauge can only be set a value, no inc/dec
// because we need both metrics2.0 gauge and counter types, but the methods of go-metrics gauge are too limited,
// we just use a go-metrics counter for everything, but we enhance it by adding an update method for simplicity.
// and we set the metrics 2.0 type via constructor methods

// also, this type optionally allows you to hook in a custom value generator to respond to Value() requests on the fly via
// a request/response channel (it is important that calling code handles these requests promptly!)

type stat struct {
	val       metrics.Counter
	valueReq  chan bool
	valueResp chan int64
}

func NewCounter(key string, customValue bool) stat {
	name := fmt.Sprintf("service_is_carbon-tagger.instance_is_%s.target_type_is_counter.%s", *stats_id, key)
	stat := stat{val: metrics.NewCounter()}
	metrics.Register(name, stat)
	if customValue {
		stat.valueReq = make(chan bool)
		stat.valueResp = make(chan int64)
	}
	return stat
}
func NewGauge(key string, customValue bool) stat {
	name := fmt.Sprintf("service_is_carbon-tagger.instance_is_%s.target_type_is_gauge.%s", *stats_id, key)
	stat := stat{val: metrics.NewCounter()}
	metrics.Register(name, stat)
	if customValue {
		stat.valueReq = make(chan bool)
		stat.valueResp = make(chan int64)
	}
	return stat
}

func (s *stat) Clear() {
	s.val.Clear()
}

func (s *stat) Count() int64 {
	if s.valueReq != nil {
		s.valueReq <- true
		val := <-s.valueResp
		s.Update(val)
	}
	return s.val.Count()
}

func (s *stat) Dec(i int64) {
	s.val.Dec(i)
}

func (s *stat) Inc(i int64) {
	s.val.Inc(i)
}

func (s *stat) Snapshot() metrics.Counter {
	return s.val.Snapshot()
}

func (s *stat) Update(v int64) {
	s.val.Clear()
	s.val.Inc(v)
}
