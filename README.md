# Carbon-tagger
## graphite protocol extension
Standard graphite protocol (`proto1`)
```
string.based.metric[...] value unix_timestamp
```
"string", "based", "metric" being the nodes of the metric.

Extended graphite protocol (backwards compatible) extends this by stating:

* nodes can be old-style values, or new-style `key=val` tag pairs
* if there's a "=" in one or more of the nodes, we'll try to parse as `proto2` and add it to the index if below conditions are met.
* there must be a tag pair with `unit` as tag key.
* there must be at least one other tag.
* you can freely choose the order of the nodes for every metric, but when you change the order, you change the metric key.
* old-style nodes (i.e. not "key=val" format) implicitly get an "nX" tag key where X is the node position in the string, starting from 1.

You'll probably want to follow the [guidelines for naming metrics](https://github.com/vimeo/graph-explorer/wiki/Consistent-tag-keys-and-values) as well.

So this defines a metric as a set of tags, and while we're at it, it also
specifies the metric_id that will be used by the current carbon and related tools.
carbon-tagger will maintain a database of metrics and their tags, and pass it on (unaltered) to a daemon
like carbon-cache or carbon-relay. So the protocol is fully backwards compatible.

This is [metrics 2.0](http://dieter.plaetinck.be/metrics_2_a_proposal.html) but using dots as delimiters until we can fix graphite.


# how does this affect the rest of my stack?

* carbon-relay, carbon-cache: unaffected, they receive the same data as usual, they need to
identify metrics by key and that stays the same.
* graphite-web: the fact that new style metrics show up in the tree of the graphite composer is more of an artifact.  It's an inferior UI model that I want to phase out.
ideally, dashboards leverage the tag database, like [graph-explorer](http://vimeo.github.io/graph-explorer) does.
* aggregators like statsd will need `proto2` support.  The added bonus here is that things actually become simpler:
we can do away with all the prefix/suffix/namespacing hacks as that all becomes moot!

# why do you process the same metrics every time they are submitted?

to have a realtime database. you could get all metricnames later and process them offline, which lowers resource usage but has higher delays

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


