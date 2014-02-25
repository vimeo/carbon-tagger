#!/bin/bash
host=$(grep -A3 elasticsearch carbon-tagger.conf | sed -n 's/^host = "\(.*\)"/\1/p')
port=$(grep -A3 elasticsearch carbon-tagger.conf | sed -n 's/^port = \(.*\)/\1/p')
index=$(grep -A3 elasticsearch carbon-tagger.conf | sed -n 's/^index = "\(.*\)"/\1/p')

if [ -z "$index" ]; then
    echo "Could not parse index from config!"
    exit 2
fi
echo "delete existing index $index (maybe)"
curl -X DELETE http://$host:$port/$index
echo
echo "create index $index"
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
