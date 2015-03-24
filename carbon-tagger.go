package main

import (
	"bufio"
	"flag"
	"fmt"
	m20 "github.com/metrics20/go-metrics20"
	"github.com/vimeo/carbon-tagger/_third_party/github.com/Dieterbe/go-metrics"
	"github.com/vimeo/carbon-tagger/_third_party/github.com/Dieterbe/go-metrics/exp"
	elastigo "github.com/vimeo/carbon-tagger/_third_party/github.com/mattbaird/elastigo/lib"
	"github.com/vimeo/carbon-tagger/_third_party/github.com/stvp/go-toml-config"
	"io"
	"net"
	"net/http"
	"os"
	"runtime/pprof"
	"strconv"
	"strings"
	"time"
)

func dieIfError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %s\n", err.Error())
		os.Exit(1)
	}
}

var (
	verbose    bool
	cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")
	memprofile = flag.String("memprofile", "", "write memory profile to this file")
	configFile = flag.String("config", "carbon-tagger.conf", "config file")

	es_host         = config.String("elasticsearch.host", "undefined")
	es_port         = config.Int("elasticsearch.port", 9200)
	es_index_name   = config.String("elasticsearch.index", "graphite_metrics2")
	es_flush_int    = config.Int("elasticsearch.flush_interval", 2)
	es_max_backlog  = config.Int("elasticsearch.max_backlog", 1000) // if this many is in transit to indexer, start blocking
	es_max_pending  = config.Int("elasticsearch.max_pending", 500)
	in_port         = config.Int("in.port", 2003)
	stats_host      = config.String("stats.host", "localhost")
	stats_port      = config.Int("stats.port", 2005)
	stats_http_addr = config.String("stats.http_addr", "0.0.0.0:8123")

	stats_id             *string
	stats_flush_interval *int

	in_conns_current             stat
	in_conns_broken_total        stat
	in_metrics_proto1_good_total stat
	in_metrics_proto2_good_total stat
	in_metrics_proto1_bad_total  stat
	in_metrics_proto2_bad_total  stat
	in_lines_bad_total           stat
	num_seen_proto2              stat
	num_seen_proto1              stat
	pending_backlog_proto1       stat // backlog in our queue (excl elastigo queue)
	pending_backlog_proto2       stat // backlog in our queue (excl elastigo queue)
	pending_es_proto1            stat
	pending_es_proto2            stat

	lines_read  chan []byte
	proto1_read chan string
	proto2_read chan m20.MetricSpec
)

func init() {
	flag.BoolVar(&verbose, "verbose", false, "print invalid lines and metrics")
}

func main() {
	flag.Parse()
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		dieIfError(err)
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
		fmt.Println("cpuprof on")
	}
	if *memprofile != "" {
		f, err := os.Create(*memprofile)
		dieIfError(err)
		defer f.Close()
		defer pprof.WriteHeapProfile(f)
	}

	stats_id = config.String("stats.id", "myhost")
	stats_flush_interval = config.Int("stats.flush_interval", 10)
	err := config.Parse(*configFile)
	dieIfError(err)

	in_conns_current = NewGauge("unit_is_Conn.direction_is_in.type_is_open", false)
	in_conns_broken_total = NewCounter("unit_is_Conn.direction_is_in.type_is_broken", false)
	in_metrics_proto1_good_total = NewCounter("unit_is_Metric.proto_is_1.direction_is_in.type_is_good", false) // no thorough check
	in_metrics_proto2_good_total = NewCounter("unit_is_Metric.proto_is_2.direction_is_in.type_is_good", false)
	in_metrics_proto1_bad_total = NewCounter("unit_is_Err.orig_unit_is_Metric.type_is_invalid.proto_is_1.direction_is_in", false)
	in_metrics_proto2_bad_total = NewCounter("unit_is_Err.orig_unit_is_Metric.type_is_invalid.proto_is_2.direction_is_in", false)
	in_lines_bad_total = NewCounter("unit_is_Err.orig_unit_is_Msg.type_is_invalid_line.direction_is_in", false)
	num_seen_proto1 = NewGauge("unit_is_Metric.proto_is_1.type_is_tracked", true)
	num_seen_proto2 = NewGauge("unit_is_Metric.proto_is_2.type_is_tracked", true)
	pending_backlog_proto1 = NewCounter("unit_is_Metric.proto_is_1.type_is_pending_in_backlog", true)
	pending_backlog_proto2 = NewCounter("unit_is_Metric.proto_is_2.type_is_pending_in_backlog", true)
	pending_es_proto1 = NewGauge("unit_is_Metric.proto_is_1.type_is_pending_in_es", true)
	pending_es_proto2 = NewGauge("unit_is_Metric.proto_is_2.type_is_pending_in_es", true)

	lines_read = make(chan []byte)
	proto1_read = make(chan string, *es_max_backlog)
	proto2_read = make(chan m20.MetricSpec, *es_max_backlog)

	// connect to elasticsearch database to store tags
	es := elastigo.NewConn()
	es.Domain = *es_host
	es.Port = strconv.Itoa(*es_port)

	indexer1 := es.NewBulkIndexer(4)
	indexer1.BulkMaxDocs = *es_max_pending
	indexer1.BufferDelayMax = time.Duration(*es_flush_int) * time.Second
	indexer1.Start()

	indexer2 := es.NewBulkIndexer(4)
	indexer1.BulkMaxDocs = *es_max_pending
	indexer1.BufferDelayMax = time.Duration(*es_flush_int) * time.Second
	indexer2.Start()

	go processInputLines()
	// 1 worker, but ES library has multiple workers
	go trackProto1(indexer1, *es_index_name)
	go trackProto2(indexer2, *es_index_name)

	statsAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", *stats_host, *stats_port))
	dieIfError(err)
	go metrics.Graphite(metrics.DefaultRegistry, time.Duration(*stats_flush_interval)*time.Second, "", statsAddr)

	// listen for incoming metrics
	addr, err := net.ResolveTCPAddr("tcp4", fmt.Sprintf(":%d", *in_port))
	dieIfError(err)
	listener, err := net.ListenTCP("tcp", addr)
	dieIfError(err)
	defer listener.Close()
	go func() {
		exp.Exp(metrics.DefaultRegistry)
		fmt.Printf("carbon-tagger %s expvar web on %s\n", *stats_id, *stats_http_addr)
		err := http.ListenAndServe(*stats_http_addr, nil)
		if err != nil {
			fmt.Println("Error opening http endpoint:", err.Error())
			os.Exit(1)
		}
	}()

	fmt.Printf("carbon-tagger %s listening on %d\n", *stats_id, *in_port)
	for {
		// would be nice to have a metric showing highest amount of connections seen per interval
		conn_in, err := listener.Accept()
		if err != nil {
			fmt.Fprint(os.Stderr, err)
			continue
		}
		go handleClient(conn_in)
	}
}

func handleClient(conn_in net.Conn) {
	in_conns_current.Inc(1)
	defer in_conns_current.Dec(1)
	defer conn_in.Close()
	reader := bufio.NewReader(conn_in)
	for {
		// TODO handle isPrefix cases (means we should merge this read with the next one in a different packet, i think)
		buf, err := reader.ReadBytes('\n')
		if err != nil {
			str := strings.TrimSpace(string(buf))
			if err != io.EOF {
				fmt.Printf("WARN connection closed uncleanly/broken: %s\n", err.Error())
				in_conns_broken_total.Inc(1)
			}
			if len(str) > 0 {
				// todo handle incomplete reads
				fmt.Printf("WARN incomplete read, line read: '%s'. neglecting line because connection closed because of %s\n", str, err.Error())
			}
			return
		}
		lines_read <- buf
	}
}

func processInputLines() {
	for buf := range lines_read {
		str := strings.TrimSpace(string(buf))
		elements := strings.Split(str, " ")
		if len(elements) != 3 {
			if verbose {
				fmt.Println("line has !=3 elements:", str)
			}
			in_lines_bad_total.Inc(1)
			continue
		}
		id := elements[0]
		if m20.IsMetric20(id) {
			metric, err := m20.NewMetricSpec(id)
			if err != nil {
				if verbose {
					fmt.Println(err)
				}
				in_metrics_proto2_bad_total.Inc(1)
			} else {
				in_metrics_proto2_good_total.Inc(1)
				proto2_read <- *metric
			}
		} else {
			err := m20.InitialValidation(id, m20.Legacy)
			if err != nil {
				if verbose {
					fmt.Println(err)
				}
				in_metrics_proto1_bad_total.Inc(1)
			} else {
				in_metrics_proto1_good_total.Inc(1)
				proto1_read <- elements[0]
			}
		}
	}
}

func trackProto1(indexer *elastigo.BulkIndexer, index_name string) {
	seenEs := make(map[string]bool)    // for ES. seen once = never need to resubmit
	seenStats := make(map[string]bool) // for stats, provides "how many recently seen?"
	for {
		select {
		case str := <-proto1_read:
			seenStats[str] = true
			if _, ok := seenEs[str]; ok {
				continue
			}
			date := time.Now()
			refresh := false // we can wait until the regular indexing runs
			metric_es := m20.MetricEs{Tags: make([]string, 0)}
			err := indexer.Index(index_name, "metric", str, "", &date, &metric_es, refresh)
			dieIfError(err)
			seenEs[str] = true
		case <-num_seen_proto1.valueReq:
			num_seen_proto1.valueResp <- int64(len(seenStats))
			seenStats = make(map[string]bool)
		case <-pending_backlog_proto1.valueReq:
			pending_backlog_proto1.valueResp <- int64(len(proto1_read))
		case <-pending_es_proto1.valueReq:
			pending_es_proto1.valueResp <- int64(indexer.PendingDocuments())
		}
	}
}

func trackProto2(indexer *elastigo.BulkIndexer, index_name string) {
	seenEs := make(map[string]bool)    // for ES. seen once = never need to resubmit
	seenStats := make(map[string]bool) // for stats, provides "how many recently seen?"
	for {
		select {
		case metric := <-proto2_read:
			seenStats[metric.Id] = true
			if _, ok := seenEs[metric.Id]; ok {
				continue
			}
			date := time.Now()
			refresh := false // we can wait until the regular indexing runs
			metric_es := m20.NewMetricEs(metric)
			err := indexer.Index(index_name, "metric", metric.Id, "", &date, &metric_es, refresh)
			dieIfError(err)
			seenEs[metric.Id] = true
		case <-num_seen_proto2.valueReq:
			num_seen_proto2.valueResp <- int64(len(seenStats))
			seenStats = make(map[string]bool)
		case <-pending_backlog_proto2.valueReq:
			pending_backlog_proto2.valueResp <- int64(len(proto2_read))
		case <-pending_es_proto2.valueReq:
			pending_es_proto2.valueResp <- int64(indexer.PendingDocuments())
		}
	}
}
