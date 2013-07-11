host="es_machine"
port=9202
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
