# Metrics-copier

TODO:
* make sure the bulk thing uses 'create' and doesn't update
* check on performance, and when looks good we probably should update carbon_tagger to write to ES directly,
* what's up with the count? goes up to 125k and then every run adds 100 more :/
* also fix those errors we're getting when this runs
* better mapping, _source, type analyzing?

if/when performance becomes an issue, consider:
* edge-ngrams/reverse edge-ngrams to do it at index time
* use prefix match with regular/reverse fields
* query_string maybe
