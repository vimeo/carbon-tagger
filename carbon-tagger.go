package main

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"github.com/stvp/go-toml-config"
	"net"
	"os"
	"strings"
)

func dieIfError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %s", err.Error())
		os.Exit(1)
	}
}

func main() {
	var (
		mysql_user              = config.String("mysql.user", "carbon_tagger")
		mysql_password          = config.String("mysql.password", "carbon_tagger_pw")
		mysql_address           = config.String("mysql.address", "undefined")
		mysql_dbname            = config.String("mysql.dbname", "carbon_tagger")
		in_port                 = config.Int("in.port", 2005)
		out_host                = config.String("out.host", "localhost")
		out_port                = config.Int("out.port", 2003)
		instance_id             = config.String("instance.id", "myhost")
		instance_flush_interval = config.Int("instance.flush_interval", 60)
	)
	err := config.Parse("carbon-tagger.conf")
	dieIfError(err)
	dsn := fmt.Sprintf("%s:%s@%s/%s?charset=utf8", *mysql_user, *mysql_password, *mysql_address, *mysql_dbname)
	fmt.Println(instance_id) // will be used later for internal metrics
	fmt.Println(instance_flush_interval)

	// connect to database to store tags
	db, err := sql.Open("mysql", dsn)
	dieIfError(err)
	defer db.Close()
	// Open doesn't open a connection. Validate DSN data:
	err = db.Ping()
	dieIfError(err)
	statement_insert_tag, err := db.Prepare("INSERT INTO tags (tag_key, tag_val) VALUES( ?, ? )")
	dieIfError(err)
	statement_select_tag, err := db.Prepare("SELECT tag_id FROM tags WHERE tag_key=? AND tag_val=?")
	dieIfError(err)
	statement_insert_metric, err := db.Prepare("INSERT INTO metrics VALUES( ? )")
	dieIfError(err)
	statement_insert_link, err := db.Prepare("INSERT INTO metrics_tags VALUES( ?, ? )")
	dieIfError(err)

	// listen for incoming metrics
	service := fmt.Sprintf(":%d", *in_port)
	addr, err := net.ResolveTCPAddr("tcp4", service)
	dieIfError(err)
	listener, err := net.ListenTCP("tcp", addr)
	dieIfError(err)
	defer listener.Close()

	// connect to outgoing carbon daemon (carbon-relay, carbon-cache, ..)
	// TODO implement fwd'ing, toggle on/off
	out := fmt.Sprintf("%s:%d", *out_host, *out_port)
	conn_out, err := net.Dial("tcp", out)
	dieIfError(err)

	for {
		fmt.Println("ready to accept")
		conn_in, err := listener.Accept()
		fmt.Println("accepted")
		dieIfError(err)
		fmt.Println("handling connection in subroutine")
		go handleClient(conn_in, conn_out, db, statement_insert_tag, statement_select_tag, statement_insert_metric, statement_insert_link)
	}
}

func parseTagBasedMetric(metric string) (metric_id string, tags map[string]string, err error) {
	fmt.Printf(">incoming: %s\n", metric)
	// metric_spec value unix_timestamp
	elements := strings.Split(metric, " ")
	metric_id = ""
	if len(elements) != 3 {
		return metric_id, nil, errors.New("metric doesn't contain exactly 3 nodes")
	}
	metric_id = elements[0]
	nodes := strings.Split(metric_id, ".")
	tags = make(map[string]string)
	// TODO make sure incoming tags are sorted
	for _, node := range nodes {
		tag := strings.Split(node, "=")
		if len(tag) != 2 {
			return metric_id, nil, errors.New("bad metric spec: each node must be a 'tag_k=tag_v' pair")
		}
		if tag[0] == "" || tag[1] == "" {
			return metric_id, nil, errors.New("bad metric spec: tag_k and tag_v must be non-empty strings")
		}

		tags[tag[0]] = tag[1]
	}
	if _, ok := tags["unit"]; !ok {
		return metric_id, nil, errors.New("bad metric spec: unit tag (mandatory) not specified")
	}
	if len(tags) < 2 {
		return metric_id, nil, errors.New("bad metric spec: must have at least one tag_k/tag_v pair beyond unit")
	}
	return
}

func forwardPacket(conn_out net.Conn, b []byte) {
	fmt.Println("be4 conn out write", string(b))
	_, err := conn_out.Write(b)
	fmt.Println("after conn out write")
	dieIfError(err) // todo something more sensible
}

func handleClient(conn_in net.Conn, conn_out net.Conn, db *sql.DB, statement_insert_tag *sql.Stmt, statement_select_tag *sql.Stmt, statement_insert_metric *sql.Stmt, statement_insert_link *sql.Stmt) {
	//    defer conn_in.Close()
	fmt.Println("handleClient begin")
	var buf [512]byte
	for {
		fmt.Println("ready to read")
		bytes, err := conn_in.Read(buf[0:])
		fmt.Println("read data")
		fmt.Println(string(buf[0:]), bytes, err)
		if err != nil {
			return
		}
		str := string(buf[0:bytes])
		if strings.ContainsAny(str, "=") {
			str = strings.TrimSpace(str)
			metric_id, tags, err := parseTagBasedMetric(str)
			if err != nil {
				fmt.Printf("DEBUG: invalid tag based metric, ignoring (%s)\n", err)
			} else {
				fmt.Printf("DEBUG: valid tag based metric %s, storing tags and forwarding\n", strings.TrimSpace(str))
				// TODO this should go in a transaction. for now we first store all tag_k=tag_v pairs (if they are orphans, it's not so bad)
				// then add the metric, than the coupling between metric and tags. <-- all this should def. be in a transaction
				tag_ids := make([]int64, 0) //maybe set cap to len(tags) or so
				for tag_k, tag_v := range tags {
					fmt.Println("Key:", tag_k, "Value:", tag_v)
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
				_, err = statement_insert_metric.Exec(metric_id)
				if err != nil {
					if err.(*mysql.MySQLError).Number != 1062 { // Error 1062: Duplicate entry 'unit=f.b=aeu' for key 'PRIMARY'
						fmt.Fprintf(os.Stderr, "can't store metric %s:%s\n", metric_id, err.Error())
						return
					}
				} else {
					// the metric is newly inserted.. we still have to couple it to the tags.
					// so we assume if the metric already existed, we don't need to do this anymore. which is not very accurate since we don't use transactions yet
					for _, tag_id := range tag_ids {
						_, err = statement_insert_link.Exec(metric_id, tag_id)
						if err != nil {
							fmt.Fprintf(os.Stderr, "can't link metric %s to tag:%s\n", metric_id, err.Error())
							return
						}
					}
				}
				forwardPacket(conn_out, buf[0:bytes])
			}
		} else {
			fmt.Printf("DEBUG: not tag based, forwarding metric %s\n", strings.TrimSpace(str))
			forwardPacket(conn_out, buf[0:bytes])
		}
	}
	fmt.Println("handleClient ending.. closing conn_in")
	conn_in.Close()
	fmt.Println("handleClient ended")
}
