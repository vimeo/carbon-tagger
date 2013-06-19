# Carbon-tagger
## graphite protocol extension
Standard graphite protocol (`proto1`)
```
string.based.metric[...] value unix_timestamp
```
Extended graphite protocol (backwards compatible), `proto2`:
```
key=val.key=val[....] value unix_timestamp
```
If the first field contains an "=", it is assumed to be `proto2`:  
the metric_id is:

* an ordered list of `key=val` tag pairs, joined by dots.
* there must be `unit` tag (with a value like `B/s`, `packets`, `queries/m` etc)
* there must be at least one other tag.
* the nodes must be ordered


So this defines a metric as a set of tags, and while we're at it, it also
specifies the metric_id that will be used by the current carbon and related tools.
carbon-tagger will maintain a database of metrics and their tags, and pass it on (unaltered) to a daemon
like carbon-cache or carbon-relay. So the protocol is fully backwards compatible.


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
currently:

# future optimisations:

* multiple sql workers
* check for indices?
* if metrics arealready in already_tracked, don't put them in the channel
* populate an sql cache from disk on program start
* infinitely sized "to track" queue that spills to disk when needed

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
