# The philosophy behind full tag-based metrics
## why tags
think about the code from where you submit metrics into statsd or graphite.
You know lots of things about the metric when you're submitting it,
and you know it in a very structured way.
You know the hostname, what unit the metric is in, and lots more "attributes".
So far, we've been compiling (dumbing down) this information into a static string.
Once the metrics are in graphite it's hard to reconstruct the original structured
information we had.  [graph-explorer](http://vimeo.github.io/graph-explorer) does
a pretty good job, but we can do better, if we maintain the structured information by
transporting metrics as a set of key-value pairs (tags), from source to destination.

Organising string based metrics into a tree was a pain, because you have to decide in advance
which dimension you'll keep the same and which will be variable in the graphing phase.
With tag-based metrics you can avoid this problem alltogether and always combine tags based
on any dimension.  (see the [graph-explorer homepage](http://vimeo.github.io/graph-explorer)
for examples).

## why is a metric a set of tags only, and not a name with a set of tags?
think about what the "name" of a metric really is. the string(s) you put in there.
"name" is pretty meaningless and only lends itself to string matching.  I would argue you can always move whatever text
you would use for a name in more clearly named tag attributes, which are far more useful.
For example if you had a metric with name "packets_received" and tags `server=mycarbonserver service=carbon`
Try to split all information about the metric in clear, atomic bits of information, like so:
`unit=packets direction=in server=mycarbonserver service=carbon`

Now you can for example easily view together all metrics with direction in, be it metrics, messages, or bits of network traffic, or all metrics that deal with packets (no matter the direction)
or use `group/sum/avg/.. by <tag>` features of your dashboard.

# carbon-tagger design choices
Standard graphite protocol:
```
string.based.metric value unix_timestamp
```
Extended graphite protocol (backwards compatible):
```
key=val.key=val[....] value unix_timestamp
```
If the first field contains an "=", it is assumed to be an ordered list of `key=val` tag pairs,
joined by dots.  So this defines a metric as a list of tags, and while we're at it, it also
specifies the metric_id that will be used to identify the metric on disk (by whisper).
carbon-tagger will maintain a database of metrics and their tags, and pass it on (unaltered) to a daemon
like carbon-cache or carbon-relay. So the protocol is fully backwards compatible.
If you submit metrics in this form, remember:

* there must be `unit` tag (with a value like `B/s`, `packets`, `queries/m` etc)
* there must be at least one other tag.
* the fields must be ordered

# how does this affect the rest of my stack?

* carbon-relay, carbon-cache: unaffected, they receive the same data as usual, they need to
identify metrics by key and stays the same.
* graphite-web: the fact that new style metrics show up in the tree of the graphite composer is more of an artifact.  It's an inferior UI model that I want to phase out.
ideally, dashboards leverage the tag database, like [graph-explorer](http://vimeo.github.io/graph-explorer) does.
* aggregators like statsd will need to be extended for the extended protocol.  The added bonus here is that things actually become simpler:
we can do away with all the prefix/suffix/namespacing hacks as that all becomes moot!

# why do you process the same metrics every time they are submitted?

to have a realtime database. you could get all metricnames and process them offline, which lowers resource usage but has higher delays

# installation

* if you already have a working Go setup, adjust accordingly:

```
mkdir -p ~/go/
export GOPATH=~/go
go get github.com/Vimeo/carbon-tagger
```

* install a database and create the tables, I test using mysql. but it should be trivial to support others.

on Centos:
```
yum install mysql mysql-server
chkconfig --levels 235 mysqld on
mysql -u root
> grant all privileges ON carbon_tagger.* TO 'carbon_tagger' IDENTIFIED BY 'carbon_tagger_pw';
> create database carbon_tagger;
> FLUSH PRIVILEGES;

mysql -h $HOST -u carbon_tagger --password=carbon_tagger_pw
> CREATE TABLE IF NOT EXISTS metrics (metric_id char(255) primary key);
> CREATE TABLE IF NOT EXISTS tags (tag_id integer primary key auto_increment, tag_key char(50), tag_val char(255));
> ALTER TABLE tags ADD CONSTRAINT UNIQUE(tag_key, tag_val); -- we rely on this! ERROR 1062 (23000): Duplicate entry 'e-c' for key 'tag_key'
> CREATE TABLE IF NOT EXISTS metrics_tags (metric_id char(255), tag_id int);
> ALTER TABLE metrics_tags ADD CONSTRAINT metric_id FOREIGN KEY (metric_id) references metrics(metric_id);
> ALTER TABLE metrics_tags ADD CONSTRAINT tag_id FOREIGN KEY (tag_id) references tags(tag_id);
```
