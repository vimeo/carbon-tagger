package main

import (
	"bufio"
	"database/sql"
	"errors"
	"fmt"
	"github.com/cactus/go-statsd-client/statsd"
	"github.com/go-sql-driver/mysql"
	"github.com/stvp/go-toml-config"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"
)

func dieIfError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %s\n", err.Error())
		os.Exit(1)
	}
}

type metricSpec struct {
	metric_id string
	tags      map[string]string
}

type Stats struct {
	mu                           sync.Mutex
	in_conns_current             int
	in_conns_broken_total        int64
	in_metrics_proto2_bad_total  int64
	in_metrics_proto2_good_total int64
	in_metrics_proto1_total      int64
	already_tracked              map[string]bool
}

func NewStats() *Stats {
	return &Stats{already_tracked: make(map[string]bool)}
}

func main() {
	var (
		mysql_user            = config.String("mysql.user", "carbon_tagger")
		mysql_password        = config.String("mysql.password", "carbon_tagger_pw")
		mysql_address         = config.String("mysql.address", "undefined")
		mysql_dbname          = config.String("mysql.dbname", "carbon_tagger")
		mysql_max_pending     = config.Int("mysql.max_pending", 1000000)
		in_port               = config.Int("in.port", 2005)
		out_host              = config.String("out.host", "localhost")
		out_port              = config.Int("out.port", 2003)
		statsd_address        = config.String("statsd.address", "localhost:8125")
		statsd_id             = config.String("statsd.id", "myhost")
		statsd_flush_interval = config.Int("statsd.flush_interval", 2003)
	)
	err := config.Parse("carbon-tagger.conf")
	dieIfError(err)
	dsn := fmt.Sprintf("%s:%s@%s/%s?charset=utf8", *mysql_user, *mysql_password, *mysql_address, *mysql_dbname)

	// connect to database to store tags
	db, err := sql.Open("mysql", dsn)
	dieIfError(err)
	defer db.Close()
	// Open doesn't open a connection. Validate DSN data:
	err = db.Ping()
	db.SetMaxIdleConns(80)
	dieIfError(err)

	// listen for incoming metrics
	addr, err := net.ResolveTCPAddr("tcp4", fmt.Sprintf(":%d", *in_port))
	dieIfError(err)
	listener, err := net.ListenTCP("tcp", addr)
	dieIfError(err)
	defer listener.Close()

	// we can queue up to max_pending: if more than that are pending flush to mysql, start blocking..
	metrics_to_track := make(chan metricSpec, *mysql_max_pending)

	// statsd client
	s, err := statsd.Dial(*statsd_address, fmt.Sprintf("carbon-tagger.%s", *statsd_id))
	dieIfError(err)
	defer s.Close()

	stats := NewStats()

	submitStats := func(stats *Stats, s *statsd.Client) {
		for {
			stats.mu.Lock()
			s.Gauge("in.conns.current", int64(stats.in_conns_current), 1)
			s.Gauge("in.conns.broken.total", stats.in_conns_broken_total, 1)
			s.Gauge("in.metrics.proto2.bad.total", stats.in_metrics_proto2_bad_total, 1)
			s.Gauge("in.metrics.proto2.good.total", stats.in_metrics_proto2_good_total, 1)
			s.Gauge("in.metrics.proto1.total", stats.in_metrics_proto1_total, 1)
			s.Gauge("metrics.proto2.count-pending-tracking", int64(len(metrics_to_track)), 1)
			s.Gauge("metrics.proto2.count-already-tracked", int64(len(stats.already_tracked)), 1)
			stats.mu.Unlock()
			time.Sleep(time.Duration(*statsd_flush_interval) * time.Second) // todo: run the stats submissions exactly every X seconds
		}
	}
	go submitStats(stats, s)

	lines_to_forward := make(chan []byte)
	// 1 forwarding worker should suffice?
	go forwardLines(*out_host, *out_port, lines_to_forward, stats, s)

	// for now, only 1 sql persist worker...
	go trackMetrics(db, metrics_to_track, stats)

	fmt.Printf("carbon-tagger %s ready to serve on %d\n", *statsd_id, *in_port)
	for {
		// would be nice to have a metric showing highest amount of connections seen per interval
		conn_in, err := listener.Accept()
		dieIfError(err)
		go handleClient(conn_in, metrics_to_track, lines_to_forward, stats)
	}
}

func parseTagBasedMetric(metric_line string) (metric metricSpec, err error) {
	// metric_spec value unix_timestamp
	elements := strings.Split(metric_line, " ")
	metric_id := ""
	if len(elements) != 3 {
		return metricSpec{metric_id, nil}, errors.New(fmt.Sprintf("metric doesn't contain exactly 3 nodes: %s", metric_line))
	}
	metric_id = elements[0]
	nodes := strings.Split(metric_id, ".")
	tags := make(map[string]string)
	// TODO make sure incoming tags are sorted
	for _, node := range nodes {
		tag := strings.Split(node, "=")
		if len(tag) != 2 {
			return metricSpec{metric_id, nil}, errors.New("bad metric spec: each node must be a 'tag_k=tag_v' pair")
		}
		if tag[0] == "" || tag[1] == "" {
			return metricSpec{metric_id, nil}, errors.New("bad metric spec: tag_k and tag_v must be non-empty strings")
		}

		tags[tag[0]] = tag[1]
	}
	if _, ok := tags["unit"]; !ok {
		return metricSpec{metric_id, nil}, errors.New("bad metric spec: unit tag (mandatory) not specified")
	}
	if len(tags) < 2 {
		return metricSpec{metric_id, nil}, errors.New("bad metric spec: must have at least one tag_k/tag_v pair beyond unit")
	}
	return metricSpec{metric_id, tags}, nil
}

func trackMetrics(db *sql.DB, metrics_to_track chan metricSpec, stats *Stats) {
	statement_insert_tag, err := db.Prepare("INSERT INTO tags (tag_key, tag_val) VALUES( ?, ? )")
	dieIfError(err)
	statement_select_tag, err := db.Prepare("SELECT tag_id FROM tags WHERE tag_key=? AND tag_val=?")
	dieIfError(err)
	statement_insert_metric, err := db.Prepare("INSERT INTO metrics VALUES( ? )")
	dieIfError(err)
	statement_insert_link, err := db.Prepare("INSERT INTO metrics_tags VALUES( ?, ? )")
	dieIfError(err)
	for {
		metric := <-metrics_to_track
		// this is racey but that's not so bad, processing the same metric and sending it to mysql twice is not so bad.
		stats.mu.Lock()
		_, ok := stats.already_tracked[metric.metric_id]
		stats.mu.Unlock()
		if ok {
			continue
		}
		// TODO this should go in a transaction. for now we first store all tag_k=tag_v pairs (if they are orphans, it's not so bad)
		// then add the metric, than the coupling between metric and tags. <-- all this should def. be in a transaction
		tag_ids := make([]int64, 0) //maybe set cap to len(tags) or so
		for tag_k, tag_v := range metric.tags {
			res, err := statement_insert_tag.Exec(tag_k, tag_v)
			if err != nil {
				if err.(*mysql.MySQLError).Number == 1062 { // Error 1062: Duplicate entry
					var id int64
					err := statement_select_tag.QueryRow(tag_k, tag_v).Scan(&id)
					if err != nil {
						fmt.Fprintf(os.Stderr, "Can't lookup the id of tag %s=%s: %s\n", tag_k, tag_v, err.Error())
						return
					}
					tag_ids = append(tag_ids, id)
				} else {
					fmt.Fprintf(os.Stderr, "can't store tag %s=%s: %s\n", tag_k, tag_v, err.Error())
					return
				}
			} else {
				id, err := res.LastInsertId()
				if err != nil {
					fmt.Fprintf(os.Stderr, "can't get id for just inserted tag %s=%s: %s\n", tag_k, tag_v, err.Error())
					return
				} else {
					tag_ids = append(tag_ids, id)
				}
			}
		}
		_, err = statement_insert_metric.Exec(metric.metric_id)
		if err != nil {
			if err.(*mysql.MySQLError).Number != 1062 { // Error 1062: Duplicate entry 'unit=f.b=aeu' for key 'PRIMARY'
				fmt.Fprintf(os.Stderr, "can't store metric %s:%s\n", metric.metric_id, err.Error())
				return
			}
		} else {
			// the metric is newly inserted.. we still have to couple it to the tags.
			// so we assume if the metric already existed, we don't need to do this anymore. which is not very accurate since we don't use transactions yet
			for _, tag_id := range tag_ids {
				_, err = statement_insert_link.Exec(metric.metric_id, tag_id)
				if err != nil {
					fmt.Fprintf(os.Stderr, "can't link metric %s to tag:%s\n", metric.metric_id, err.Error())
					return
				}
			}
		}
		stats.mu.Lock()
		stats.already_tracked[metric.metric_id] = true
		stats.mu.Unlock()
	}
}

// from https://gist.github.com/moraes/2141121
type Node struct {
	line []byte
}

// Queue is a basic FIFO queue based on a circular list that resizes as needed.
type Queue struct {
	nodes []*Node
	size  int
	head  int
	tail  int
	count int
}

// NewQueue returns a new queue with the given initial size.
func NewQueue(size int) *Queue {
	return &Queue{
		nodes: make([]*Node, size),
		size:  size,
	}
}

// Push adds a node to the queue.
func (q *Queue) Push(n *Node) {
	if q.head == q.tail && q.count > 0 {
		nodes := make([]*Node, len(q.nodes)+q.size)
		copy(nodes, q.nodes[q.head:])
		copy(nodes[len(q.nodes)-q.head:], q.nodes[:q.head])
		q.head = 0
		q.tail = len(q.nodes)
		q.nodes = nodes
	}
	q.nodes[q.tail] = n
	q.tail = (q.tail + 1) % len(q.nodes)
	q.count++
}

// Pop removes and returns a node from the queue in first to last order.
func (q *Queue) Pop() *Node {
	if q.count == 0 {
		return nil
	}
	node := q.nodes[q.head]
	q.head = (q.head + 1) % len(q.nodes)
	q.count--
	return node
}

func forwardLines(out_host string, out_port int, lines_to_forward chan []byte, stats *Stats, s *statsd.Client) {
	// connect to outgoing carbon daemon (carbon-relay, carbon-cache, ..)
	conn_out, err := net.Dial("tcp", fmt.Sprintf("%s:%d", out_host, out_port))
	dieIfError(err)
	lines_to_retry := NewQueue(100)

	out_lines_error := int64(0)
	out_lines_ok := int64(0)
	var line []byte
	for {
		if lines_to_retry.count > 0 {
			// copy needed?
			copy(line, lines_to_retry.Pop().line)
		} else {
			line = <-lines_to_forward
		}
		_, err := conn_out.Write(line)
		if err != nil {
			// asert err.(*net.OpError) without panicking?
			if err.(*net.OpError).Err == syscall.EPIPE {
				// TODO for now this just blocks.. later we'll want lines_to_retry to become a real FIFO and don't block input
				// (upto a certain max size).  then, instrument how long the recovery takes (in time and in requeued lines)
				fmt.Fprintf(os.Stderr, "broken pipe to %s:%d. reconnecting..\n", out_host, out_port)
				conn_out, err = net.Dial("tcp", fmt.Sprintf("%s:%d", out_host, out_port))
				for err != nil && err.(*net.OpError).Err == syscall.ECONNREFUSED {
					fmt.Fprintf(os.Stderr, "conn refused @ %s:%d. reconnecting..\n", out_host, out_port)
					time.Sleep(500 * time.Millisecond)
					conn_out, err = net.Dial("tcp", fmt.Sprintf("%s:%d", out_host, out_port))
				}
				fmt.Fprintf(os.Stderr, "reconnected to %s:%d.\n", out_host, out_port)
				lines_to_retry.Push(&Node{line})
			} else {
				out_lines_error += 1
				s.Gauge("out.lines.error", out_lines_error, 1)
			}
		} else {
			out_lines_ok += 1
			s.Gauge("out.lines.ok", out_lines_ok, 1)
		}
	}
}

func handleClient(conn_in net.Conn, metrics_to_track chan metricSpec, lines_to_forward chan []byte, stats *Stats) {
	stats.mu.Lock()
	stats.in_conns_current += 1
	stats.mu.Unlock()
	defer conn_in.Close()
	reader := bufio.NewReader(conn_in)
	for {
		// TODO handle isPrefix cases (means we should merge this read with the next one in a different packet, i think)
		buf, err := reader.ReadBytes('\n')
		if err != nil {
			str := strings.TrimSpace(string(buf))
			if err != io.EOF {
				fmt.Printf("WARN connection closed uncleanly/broken: %s\n", err.Error())
				stats.mu.Lock()
				stats.in_conns_broken_total += 1
				stats.mu.Unlock()
			}
			if len(str) > 0 {
				// todo handle incomplete reads
				fmt.Printf("WARN incomplete read, line read: '%s'. neglecting line because connection closed because of %s\n", str, err.Error())
			}
			stats.mu.Lock()
			stats.in_conns_current -= 1
			stats.mu.Unlock()
			return
		}
		str := string(buf)
		if strings.ContainsAny(str, "=") {
			str = strings.TrimSpace(str)
			metric, err := parseTagBasedMetric(str)
			if err != nil {
				stats.mu.Lock()
				stats.in_metrics_proto2_bad_total += 1
				stats.mu.Unlock()
			} else {
				stats.mu.Lock()
				stats.in_metrics_proto2_good_total += 1
				stats.mu.Unlock()
				metrics_to_track <- metric
				lines_to_forward <- buf
			}
		} else {
			stats.mu.Lock()
			stats.in_metrics_proto1_total += 1
			stats.mu.Unlock()
			lines_to_forward <- buf
		}
	}
	stats.mu.Lock()
	stats.in_conns_current -= 1
	stats.mu.Unlock()
}
