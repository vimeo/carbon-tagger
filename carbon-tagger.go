package main

import (
	"net"
	"os"
	"fmt"
    "strings"
    "errors"
    "database/sql"
    "github.com/go-sql-driver/mysql"
)

func dieIfError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %s", err.Error())
		os.Exit(1)
	}
}

func main() {

    // connect to database to store tags
    db, err := sql.Open("mysql", "carbon_tagger:carbon_tagger_pw@tcp(graphitemachine:3306)/carbon_tagger?charset=utf8")
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
	service := ":2003"
	addr, err := net.ResolveTCPAddr("tcp4", service)
	dieIfError(err)
	listener, err := net.ListenTCP("tcp", addr)
	dieIfError(err)

    // TODO connect to outgoing carbon-relay or carbon-cache

	for {
		conn_in, err := listener.Accept()
		if err != nil {
			continue
		}
		handleClient(conn_in, db, statement_insert_tag, statement_select_tag, statement_insert_metric, statement_insert_link)
		conn_in.Close()
	}
}

func parseTagBasedMetric(metric string) (metric_id string, tags map[string]string, err error) {
    fmt.Printf(">incoming: %s\n", metric)
    // metric_spec value unix_timestamp
    elements := strings.Split(metric, " ")
    metric_id = ""
    if len(elements) != 3  {
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
    if _,ok := tags["unit"]; !ok {
        return metric_id, nil, errors.New("bad metric spec: unit tag (mandatory) not specified")
    }
    if len(tags) < 2 {
        return metric_id, nil, errors.New("bad metric spec: must have at least one tag_k/tag_v pair beyond unit")
    }
    return
}


func forwardMetric(metric string) {
    // forward
    //_, err2 := conn_out.Write(buf[0:n])
    //if err2 != nil {
    //    return
   // }
}

func handleClient(conn_in net.Conn, db *sql.DB, statement_insert_tag *sql.Stmt, statement_select_tag *sql.Stmt, statement_insert_metric *sql.Stmt, statement_insert_link *sql.Stmt) {
	var buf [512]byte
	for {
		bytes, err := conn_in.Read(buf[0:])
		if err != nil {
			return
		}
        str := string(buf[0:bytes])
        if(strings.ContainsAny(str, "=")) {
            str = strings.TrimSpace(str)
            metric_id, tags, err:= parseTagBasedMetric(str)
            if err != nil {
                fmt.Printf("DEBUG: invalid tag based metric, ignoring (%s)\n", err)
            } else {
                fmt.Printf("DEBUG: valid tag based metric %s, storing tags and forwarding\n", strings.TrimSpace(str))
                // TODO this should go in a transaction. for now we first store all tag_k=tag_v pairs (if they are orphans, it's not so bad)
                // then add the metric, than the coupling between metric and tags. <-- all this should def. be in a transaction
                tag_ids := make([]int64, 1)
                for tag_k, tag_v := range tags {
                    fmt.Println("Key:", tag_k, "Value:", tag_v)
                    res, err := statement_insert_tag.Exec(tag_k, tag_v)
                    if err != nil {
                        if err.(*mysql.MySQLError).Number == 1062 {  // Error 1062: Duplicate entry
                            res, err := statement_select_tag.Exec(tag_k, tag_v)
                            if err != nil {
                                fmt.Fprintf(os.Stderr, "Can't lookup the id of tag %s=%s: %s\n", tag_k, tag_v, err.Error())
                                return
                            }
                        } else {
                            fmt.Fprintf(os.Stderr, "can't store tag %s=%s: %s\n", tag_k, tag_v, err.Error())
                            return
                        }
                    } else {
                        id, err := res.LastInsertId()
                        if err != nil {
                            tag_ids = append(tag_ids, id)
                        } else {
                            fmt.Fprintf(os.Stderr, "can't get id for just inserted tag %s=%s: %s\n", tag_k, tag_v, err.Error())
                            return
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
                forwardMetric(str)
            }
        } else {
            fmt.Printf("DEBUG: not tag based, forwarding metric %s\n", strings.TrimSpace(str))
            forwardMetric(str)
        }
	}
}

