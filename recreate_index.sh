#!/bin/bash
host=$(grep -A3 elasticsearch carbon-tagger.conf | sed -n 's/^host = "\(.*\)"/\1/p')
port=$(grep -A3 elasticsearch carbon-tagger.conf | sed -n 's/^port = \(.*\)/\1/p')
index=graphite_metrics

echo "delete any existing index (maybe)"
curl -X DELETE http://$host:$port/$index
echo
echo "create index"
curl -XPOST $host:$port/$index -d '{
    "settings" : {
        "number_of_shards" : 1
    },
    "mappings" : {
        "metric" : {
            "_source" : { "enabled" : true },
            "_id": {"index": "not_analyzed", "store" : "yes"},
            "properties" : {
                "tags" : {"type" : "string", "index" : "not_analyzed" }
            }
        }
    }
}'
echo
