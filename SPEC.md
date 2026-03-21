= OTEL-OQL

This is a service that basically does two tasks:
- It takes data in OpenTelemetry (OTEL) format on port 4317/4318 and sends it to a backend.
  backend.
- It allows to query the backend via a new query language, OQL (more later)
- The backend is Apache Pinot and can be assumed to be up and running, but does not 
  have any schema or the like
- The service will translate OQL in SQL in the Pino language and execute it.
- The service is multitenant. Incoming data will have a `tenant-id` property. If not it needs to be
  rejected. 
- The service can have a test mode, that sets a default `tenant-id` of 0.
- The service is written in Golang.
- The service will accpet all 3 kind of OTel signals: metrics, logs and traces
- The service will allow to set up the database tables/schemas/...

== OQL query language

The language will allow to select a signal to start with and execute a qury like the following:
Note, that the examples have pipes (`|`), but that is not a hard requirement.

```
signal=spans 
| where name == "checkout_process" and duration > 500ms 
| limit 1 
| expand trace  // Magic operator: fetches all spans with the same trace_id
```

This will fetch a span from a resource 'checkout_proces' that has a duration of more than 500ms

```
signal=spans
| where name == "payment_gateway" and attributes.error == "true"
| correlate logs, metrics
```

To search for a span with an error attribute and then find matching logs and metrics

```
// 1. Start in Aggregation Space (Metrics)
signal=metrics | where name == "http.server.duration" and value > 2s

// 2. Cross into Event Space via Exemplars
| get_exemplars() // Pulls the trace_ids attached to these slow metrics

// 3. Expand into Traces and Logs
| expand trace
| correlate logs
```

This is how a latency spike would be debugged:

```
// 1. Find the latency spike in the metrics
signal=metrics 
| where metric_name == "http.server.duration" and value > 5000ms

// 2. Extract the wormhole key (the Exemplar)
| extract exemplar.trace_id as bad_trace

// 3. Jump from Aggregation Space back to Event Space
| switch_context signal=spans
| where trace_id == bad_trace
| expand trace  // Rebuild the full waterfall of that specific 8-second request

// 4. (Optional) Pull the logs for that exact trace
| correlate logs
| where attributes.error == "true"
```

* If a query was executed, it must be possible to either start a new fresh query or to use
the result set for further refinement.
E.g. 

```
signal=traces
where attribute.duration > 5s
```
this will give many traces. Then in the next step

```
filter attribute.error = true
```
to filter the result set of the first query to traces with an error.

* Likewise it should be possible to expand the context:

```
signal=metrics 
| where metric_name == "http.server.duration" and value > 5000ms
| extract exemplar.trace_id as bad_trace
```

and then 

```
find baseline for bad_trace.serivce
```


