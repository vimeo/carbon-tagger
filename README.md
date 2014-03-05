# Carbon-tagger
## graphite protocol extension
Standard graphite protocol (`proto1`)
```
string.based.metric[...] value unix_timestamp
```
"string", "based", "metric" being the nodes of the metric, and metric names usually being unorganized, unstandardized and lacking information.

Carbon-tagger implements [metrics 2.0](http://dieter.plaetinck.be/metrics_2_a_proposal.html) which aims to make metrics entirely self describing, structured, and standardized.

It does this by using an axtended graphite protocol (backwards compatible):

* nodes can be old-style values, or new-style `key=val` tag pairs
* if there's a "=" in one or more of the nodes, we'll try to parse as `proto2` and add it to the index if below conditions are met.
* there must be a tag pair with `unit` as tag key.
* there must be at least one other tag.
* you can freely choose the order of the nodes for every metric, but when you change the order, you change the metric key.
* old-style nodes (i.e. not "key=val" format) implicitly get an "nX" tag key where X is the node position in the string, starting from 1.

You'll probably want to follow the [metrics naming conventions](https://github.com/vimeo/graph-explorer/wiki/Consistent-tag-keys-and-values),
specifically [apply the correct units](https://github.com/vimeo/graph-explorer/wiki/Units-%26-Prefixes)

So this defines a metric as a set of tags, and while we're at it, it also
specifies the metric_id that will be used by the current carbon and related tools.
carbon-tagger will maintain a database of metrics and their tags, and pass it on (unaltered) to a daemon
like carbon-cache or carbon-relay. So the protocol is fully backwards compatible.

This is [metrics 2.0](http://dieter.plaetinck.be/metrics_2_a_proposal.html) but
* using dots as delimiters until we can fix graphite.
* you can use units like "Mbps" or "Errps" to mean "Mb/s" and "Err/s".  Graphite treats slashes as delimiters. Carbon-tagger will set the 
  proper unit tag.



# how does this affect the rest of my stack?

* carbon-relay, carbon-cache: unaffected, they receive the same data as usual, the identifiers just look a little different.
* graphite-web: unaffected. metrics 2.0 still show up in the tree of the graphite composer, although that's an inferior UI model that I want to phase out.
ideally, dashboards leverage the tag database, like [graph-explorer](http://vimeo.github.io/graph-explorer) does.
* aggregators like statsd need `proto2` support.  [statsdaemon](https://github.com/vimeo/statsdaemon) is a drop-in statsd replacement
that for metrics in metrics2.0 format correctly represents its aggregations and statistical summaries by updating the key/value pairs of metric names, as opposed to the traditional prefix/suffix "features" which leave metrics even vaguer than they were.


# why do you process the same metrics every time they are submitted?

to have a realtime database. you could get all metricnames later and process them offline, which lowers resource usage but has higher delays

# internal metrics

are in proto2 format and are also tracked in ES automatically. they are also available through the admin interface

# admin telnet interface

commands:

```
    help         show this menu
    seen_proto1  list all proto1 metric keys seen so far
    seen_proto2  list all proto2 metric keys seen and sent to ES so far
    stats        show internal metrics performance stats
```

# performance

currently, not very optimized at all! but it's probably speedy enough,
and there's a big buffer that smoothens the effect of new metrics

* I reach about 15k metrics/s processing speed, even when the temp buffer is full and it's syncing to ES.
in fact, i don't see a discernable difference between buffer full (unblocked) and buffer full (blocked)
probably carbon-cache (whisper) was being the bottleneck?

* space used: 176B/metric (21M for 125k metrics, twice that if we'd enable indexing/analyzing)

# TODO
* make sure the bulk thing uses 'create' and doesn't update/replace the doc every time
* it seems like ES doesn't contain _all_ metrics (on 2M unique inserts, ES' count is 1889300)
* better mapping, _source, type analyzing?

# future optimisations

* if metrics are already in already_tracked, don't put them in the channel
* populate an ES cache from disk on program start
* infinitely sized "to track" queue that spills to disk when needed
* forward_lines channel buffering, so that runtime doesn't have to switch Gs all the time?
* GOMAXPROCS

if/when ES performance becomes an issue, consider:
* edge-ngrams/reverse edge-ngrams to do it at index time
* use prefix match with regular/reverse fields
* query_string maybe

# building

if you already have a working Go setup, adjust accordingly:

```
mkdir -p ~/go/
export GOPATH=~/go
go get github.com/Vimeo/carbon-tagger
```
# installation

* just copy the carbon-tagger binary and run it (TODO: initscripts)
* install elasticsearch and run it (super easy, see http://www.elasticsearch.org/guide/reference/setup/installation/, just set a unique cluster name)
* ./recreate_index.sh


