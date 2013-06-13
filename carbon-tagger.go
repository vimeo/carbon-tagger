package main

import (
	"net"
	"os"
	"fmt"
    "strings"
    "errors"
    "database/sql"
    _ "github.com/go-sql-driver/mysql"
)

func dieIfError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %s", err.Error())
		os.Exit(1)
	}
}

func main() {

    // connect to database to store tags
    db, e := sql.Open("graphite_host", "carbon_tagger:carbon_tagger_pw@/carbon_tagger?charset=utf8")
    defer db.Close()
    // Open doesn't open a connection. Validate DSN data:
    err := db.Ping()
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
		handleClient(conn_in, db)
		conn_in.Close()
	}
}

func parseTagBasedMetric(metric string) (tags map[string]string, err error) {
    fmt.Printf(">incoming: %s\n", metric)
    // metric_spec value unix_timestamp
    elements := strings.Split(metric, " ")
    if len(elements) != 3  {
        return nil, errors.New("metric doesn't contain exactly 3 nodes")
    }
    nodes := strings.Split(elements[0], ".")
    tags = make(map[string]string)
    // TODO make sure incoming tags are sorted
    for _, node := range nodes {
        tag := strings.Split(node, "=")
        if len(tag) != 2 {
            return nil, errors.New("bad metric spec: each node must be a 'tag_k=tag_v' pair")
        }
        if tag[0] == "" || tag[1] == "" {
            return nil, errors.New("bad metric spec: tag_k and tag_v must be non-empty strings")
        }

        tags[tag[0]] = tag[1]
    }
    if _,ok := tags["unit"]; !ok {
        return nil, errors.New("bad metric spec: unit tag (mandatory) not specified")
    }
    if len(tags) < 2 {
        return nil, errors.New("bad metric spec: must have at least one tag_k/tag_v pair beyond unit")
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

func handleClient(conn_in net.Conn, db DB) {
	var buf [512]byte
	for {
		bytes, err := conn_in.Read(buf[0:])
		if err != nil {
			return
		}
        str := string(buf[0:bytes])
        if(strings.ContainsAny(str, "=")) {
            str = strings.TrimSpace(str)
            tags, err:= parseTagBasedMetric(str)
            if err != nil {
                fmt.Printf("DEBUG: invalid tag based metric, ignoring (%s)\n", err)
            } else {
                fmt.Printf("DEBUG: valid tag based metric %s, storing tags and forwarding\n", strings.TrimSpace(str))
                // TODO this should go in a transaction. for now we first store all tag_k=tag_v pairs (if they are orphans, it's not so bad)
                // then add the metric, than the coupling between metric and tags. <-- all this should def. be in a transaction
                stmtIns, err := db.Prepare("INSERT INTO tags (tag_key, tag_val) VALUES( ?, ? )")
                if err != nil {
                    fmt.Fprintf(os.Stderr, "db prepare failed\n")
                    return
                }
                tag_ids := make([]int, 1)
                for tag_k, tag_v := range tags {
                    fmt.Println("Key:", tag_k, "Value:", tag_v)
                    _, err = stmtIns.Exec(tag_k, tag_v)
                    if err != nil {
                        fmt.Fprintf(os.Stderr, "can't store tag %s=%s\n", tag_k, tag_v)
                        return
                    }
                    // TODO on ERROR 1062 (23000), select id
                    id := nil
                    id, err = res.LastInsertId()
                    if err != nil {
                        append(tag_ids, id)
                    } else {
                        fmt.Fprintf(os.Stderr, "can't store tag %s=%s\n", tag_k, tag_v)
                        return
                    }
                }
                stmtIns, err := db.Prepare("INSERT INTO metrics VALUES( ? )")
                if err != nil {
                    fmt.Fprintf(os.Stderr, "db prepare failed\n")
                    return
                }
                _, err = stmtIns.Exec(str)
                if err != nil {
                    fmt.Fprintf(os.Stderr, "db insert failed\n")
                    return
                }
                stmtIns, err := db.Prepare("INSERT INTO metrics_tags VALUES( ?, ? )")
                if err != nil {
                    fmt.Fprintf(os.Stderr, "db prepare failed\n")
                    return
                }
                for _, tag_id := range tag_ids {
                    _, err = stmtIns.Exec(str, tag_id)
                    if err != nil {
                        fmt.Fprintf(os.Stderr, "db insert failed\n")
                        return
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

