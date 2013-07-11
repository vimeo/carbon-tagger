package main

import (
	"database/sql"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"github.com/mattbaird/elastigo/api"
	"github.com/mattbaird/elastigo/core"
	"github.com/stvp/go-toml-config"
	"os"
	"time"
)

func dieIfError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %s\n", err.Error())
		os.Exit(1)
	}
}

func main() {
	var (
		mysql_user     = config.String("mysql.user", "carbon_tagger")
		mysql_password = config.String("mysql.password", "carbon_tagger_pw")
		mysql_address  = config.String("mysql.address", "undefined")
		mysql_dbname   = config.String("mysql.dbname", "carbon_tagger")
	)
	err := config.Parse("metrics-copier.conf")
	dieIfError(err)
	dsn := fmt.Sprintf("%s:%s@%s/%s?charset=utf8", *mysql_user, *mysql_password, *mysql_address, *mysql_dbname)

	db, err := sql.Open("mysql", dsn)
	dieIfError(err)
	defer db.Close()
	// Open doesn't open a connection. Validate DSN data:
	err = db.Ping()
	db.SetMaxIdleConns(80)
	dieIfError(err)

	// ES
	api.Domain = "es_machine"
	api.Port = "9202"
	done := make(chan bool)
	core.BulkIndexorGlobalRun(4, done)

	rows, err := db.Query("SELECT metrics_tags.metric_id, tags.tag_key, tags.tag_val FROM metrics_tags, tags WHERE metrics_tags.tag_id = tags.tag_id ORDER BY metrics_tags.metric_id")
	if err != nil {
		fmt.Println("mysql error %i", err.(*mysql.MySQLError).Number)
	}
	dieIfError(err)
	type Metric struct {
		Tags []string `json:"tags"`
	}
	var metric_id string
	var last_metric_id string
	var tag_key string
	var tag_val string
	// this could be more efficient in two ways:
	// don't append (expensive resize)
	// don't keep creating a new one for every metric, reuse same datastructure
	// we could for example keep an array with like 100 slots (no metric will ever reach that much) and fill the array from 0 upwards,
	// and then just create a slice pointing to the last inserted item.
	tags := make([]string, 0)
	for rows.Next() {
		err = rows.Scan(&metric_id, &tag_key, &tag_val)
		fmt.Printf("read: %s %s=%s\n", metric_id, tag_key, tag_val)
		if last_metric_id == "" {
			fmt.Println("initializing last_metric_id for the very first time")
			last_metric_id = metric_id
		}
		if metric_id != last_metric_id {
			fmt.Println("oh! we just started a new metric")
			date := time.Now()
			metric := Metric{tags}
			fmt.Printf("saving metric %s - %s", last_metric_id, metric)
			err = core.IndexBulk("graphite_metrics", "metric", last_metric_id, &date, &metric)
			dieIfError(err)
			tags = make([]string, 0)
			fmt.Printf("tags is now %s (should be empty)\n", tags)
			last_metric_id = metric_id
		}
		tags = append(tags, fmt.Sprintf("%s=%s", tag_key, tag_val))
		fmt.Printf("tags is now %s\n", tags)
	}
	err = rows.Err()
	dieIfError(err)
	//func Index(pretty bool, index string, _type string, id string, data interface{}) (api.BaseResponse, error) {
	//response, err := core.Index(true, "graphite_metrics", "metric", id +"?op_type=create",  NewTweet("kimchy", "Search is cool"))
	//url = fmt.Sprintf("/%s/%s/%s?%s", index, _type, id, api.Pretty(pretty))
}
