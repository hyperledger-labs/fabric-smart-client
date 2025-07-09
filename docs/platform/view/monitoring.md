# Monitoring

## Overview

We use the following two methods to monitor the performance of the application:
* **Metrics** provide an overview of the overall system performance using aggregated results, e.g. total requests, requests per second, current state of a variable, average duration, percentile of duration
* **Traces** help us analyze single requests by breaking down their lifecycles into smaller components

## Metrics

There are three types of metrics:
```go
type Counter interface {
	With(labelValues ...string) Counter
	Add(delta float64)
}
type Gauge interface {
	With(labelValues ...string) Gauge
	Add(delta float64)
	Set(value float64)
}
type Histogram interface {
	With(labelValues ...string) Histogram
	Observe(value float64)
}
```

For more information on their meaning and how they are used, refer to [this article](https://prometheus.io/docs/concepts/metric_types/).

### Types of providers

A metric provider specifies the implementation of the aforementioned metrics, and it implements the following interface:
```go
// A Provider is an abstraction for a metrics provider. It is a factory for
// Counter, Gauge, and Histogram meters.
type Provider interface {
	// NewCounter creates a new instance of a Counter.
	NewCounter(CounterOpts) Counter
	// NewGauge creates a new instance of a Gauge.
	NewGauge(GaugeOpts) Gauge
	// NewHistogram creates a new instance of a Histogram.
	NewHistogram(HistogramOpts) Histogram
}
```
FSC provides three provider implementations (custom ones can also be added), that can be configured in the `core.yaml`:
* `disabled.Provider`: No metrics are registered, when `fsc.metrics.provider = disabled`
* `statsd.Provider` when `fsc.metrics.provider = statsd`
* `prometheus.Provider`: The default implementation, when `fsc.metrics.provider = prometheus`

The default implementation of the metric provider is defined in `view.SDK` and can be overridden by any parent SDK as follows:
```go
func (p *SDK) Install() error {
	...
	p.C.Decorate(func (_ metrics.Provider, dep1 Dep1, dep2 Dep2) metrics.Provider {
		return ...
	})
	...
}
```

### Adding metrics

#### In a Service
To record metrics within a service:
* Create a new struct `Metrics` that contains as properties all the metrics this specific service and its sub-components need. Avoid adding more fields to the struct.
```go
type Metrics struct {
	RequestsSent      metrics.Counter
	RequestsReceived  metrics.Counter
	RequestsPending   metrics.Gauge
	RequestDuration   metrics.Histogram
}
```
* The constructor of `Metrics` will *only* accept the `metrics.Provider`.
```go
func NewMetrics(provider metrics.Provider) *Metrics {
	return &Metrics {
		RequestsSent:     provider.NewCounter(metrics.CounterOpts{
			Namespace:    "view", 
			Name:         "requests_sent", 
			Help:         "The number of view requests that have been received.", 
			LabelNames:   []string{"command"}, 
			StatsdFormat: "%{#fqname}.%{command}.",
		}), 
		RequestsReceived: provider.NewCounter(...), 
		RequestsPending:  provider.NewGauge(...), 
		RequestDuration:  provider.NewHistogram(...),
	}
}
```
* In the service constructor, inject the `metrics.Provider` and create the `Metrics` struct.
```go
type MyService {
  ...
  Metrics *Metrics
  ...
}

func NewMyService(... Deps, metricsProvider metrics.Provider) *MyService {
	return &MyService{
		...
		Metrics: NewMetrics(metricsProvider), 
		...
	}
}
```

#### In a View
Views do not support proper dependency injection and instantiating a new Metrics struct can be inefficient for each view invocation. For this reason, avoid keeping metrics within the views and try to do it using the services that the view uses.

However, if it is necessary, you can create a `Metrics` struct that contains all the metrics your application uses as shown above, and register it in your SDK, as explained [here](../sdk.md#developing-new-sdks):
```go
type Metrics struct {...}

func NewMetrics(provider metrics.Provider) *Metrics {...}

func (p *SDK) Install() error {
	...
	p.C.Provide(NewMetrics)
	...
	digutils.Register[NewService1](p.Container())
	...
}
```

### Visualization

The visualization of the prometheus metrics can be done in three different steps:
* **Prometheus Exporter:** The application collects the metrics and exposes them using a Prometheus exporter server that runs by default on `fsc.web.address`. To access them, you can open on your browser `http://{{fsc.web.address}}/metrics`.
* **Prometheus Server:** A separate server can be run on the same or a different host that scrapes the data from the endpoint we explained above and presents aggregated results. The fastest way is using the Docker image `prom/prometheus:latest` and adding in the `prometheus.yaml` configuration file the endpoints that need to be scraped. The Prometheus server keeps a local database to record the history of the scraped metrics. The server provides a simple UI, as well as a REST interface to query the aggregated metrics.
* **Grafana UI:** On top of the Prometheus server we can use a separate UI that issues queries to the Prometheus server and visualizes the results in more complex graphs. Additionally, dashboard pages can be defined to collect different graphs. The fastest way is using the Docker image `grafana/grafana:latest`, where the endpoint of the Prometheus server is passed in the configuration as a datasource, and the various dashboards are mounted on the image, so they don't need to be configured each time.

## Traces

Traces are used to capture, analyze and break down the lifecycle of a request that spans across different hosts. FSC uses the OpenTelemetry open standard (more information and introduction to concepts can be found [here](https://opentelemetry.io/docs/concepts/observability-primer/)).

### Types of providers

The tracer provider specifies the behavior of the traces, e.g. where they are stored, the sampling rate, the format etc.

```go
type TracerProvider interface {
	Tracer(name string, options ...TracerOption) Tracer
}
```


FSC defines the following implementations, although custom implementations can be added:

* **NoOp:** Disables the tracer, when `fsc.tracing.provider = none`
* **Console:** Exports (prints) the traces on the console, when `fsc.tracing.provider = console`
* **File:** Exports (stores) the traces into the file `fsc.tracing.file.path`, when `fsc.tracing.provider = file`
* **OTPL:** Exports (sends) the traces to an OTPL collector that listens on the port `fsc.tracing.optl.address` when `fsc.tracing.provider = optl`

These supported tracer providers can be instantiated using `tracing.NewTracerProviderFromConfig`.
Using this constructor, along with the type of the exporter, we can also specify other parameters, like the sampling rate.
The default implementation is defined in the view SDK, but can be overridden in the same way as the metrics provider.

FSC provides also an enhanced implementation of the `tracing.Provider` that keeps metrics in the background for each span.
Whenever we start spans, we keep track of the count and the duration of the operations. To use it, override the existing tracer provider with this implementation using `tracing.NewTracerProviderWithBackingProvider`.

### Adding traces

The base unit of the traces are the spans. A span represents a unit of work. In our code, we only start new spans and then these spans are linked to compose traces by the remote agent (in case we use OTPL as provider).

1. For instance, in the case of a transfer, the transfer operation is conceptually a trace. The first span of this trace can be created on the end user that initiates the client (e.g. alice).
2. When the request arrives to the recipient (e.g. bob), another span can be initiated when the corresponding view is invoked.
3. This view calls other views and/or services, each of which can initiate new spans (or re-use the same span).
4. Then bob asks his FSC peers for endorsements, and that can initiate new spans respectively.

In order to link the different spans to compose a single trace, the span metadata (span context) have to be propagated from one request to the other (i.e. from alice to bob, from bob to the intermediaries, from bob back to alice).
This happens automatically by passing the context from one unit of work to the other.

We have the following main cases where the span context has to be transmitted:
1. Within the same FSC node (case 3 in the above example). These calls can be performed in the following ways:
   1. calling a view
   2. calling a service
2. During remote calls (cases 1, 2, 4 in the above example). These calls can be performed in the following ways:
   1. calling a view using GRPC/REST (case 1, when alice calls the view on bob)
   2. calling a RESTful API directly
   3. using sessions (case 3, when bob collects endorsements using the session factory)

For all the above cases (except 1.ii), the span-context propagated is taken care of in the background by the application.
The developer must however make sure that the context is propagated between invocations of services.

#### Add Events

**You will not need to add new spans, but only add events to the existing spans.**

* The simplest way to add an event to an existing context is using the loggers methods.
In the following cases, both events will be added, but the log will be logged only if we have debug logs enabled.
```go
logger.DebugfContext(ctx, "here is a debug event")
logger.InfofContext(ctx, "here is an info event.")
```

* Otherwise, for more control and flexibility, you can retrieve the span from the context and add the event:
```go
span := trace.SpanFromContext(ctx)
span.AddEvent("here is an event")
```

#### Add traces and spans

It will not be necessary to add new traces or spans, so you can skip this part.
However, if this is indeed necessary, below some implementation details on how to add traces in services and views.

##### Add traces/spans in a Service

To start new spans within a service:
* Add the `trace.Tracer` as a field of your service struct
```go
type MyService struct {
	...
	tracer trace.Tracer
	...
	otherService OtherService
	...
}
```
* Inject the `tracing.Provider` into your service constructor and instantiate a new tracer. As the name for the tracer, try to pick a noun that describes of the actor:
```go
const (
	currencyLabel tracing.LabelName = "currency"
	successLabel tracing.LabelName = "success"
	validatorTypeLabel tracing.LabelName = "validator_type"
)

func NewMyService(... Deps, tracerProvider tracing.Provider, otherService OtherService) *MyService {
	return &MyService {
		...
		tracer: p.Tracer("transfer_responder", tracing.WithMetricsOpts(tracing.MetricsOpts{
			Namespace:  "fsc",
			LabelNames: []tracing.LabelName{currencyLabel, cuccessLabel, validatorTypeLabel},
		})),
		...
	}
}
```
* Start a new span in the method:
  * Make sure the methods of your service accept the context in their signature.
  * All the services you call need to be passed the child context `newCtx`. Otherwise you will still get the spans, but they will not belong to the same trace as the parent span.
  * As a span name, use a noun that describes the action.
  * The label values can be overwritten
  * Make sure you always `End()` the span.
  * Use `trace.SpanKindInternal` that corresponds to calls within the same node.
  * Start a new trace and avoid re-using the trace of the parent if you think that a new unit of work has started. If you need to reuse the parent span (just to add events and properties, but not to end it), use `trace.SpanFromContext(ctx)`.
  * Optionally add more events that appear as logs within your span. Prefer verbs as event names.
  * Do not try to set labels that you haven't defined in the `LabelNames` field above, and make sure all labels have a value (pick a default value if you are unsure and overwrite it later). The span implementation will not fail, but custom extensions of it (read below) will require that they all be defined and set.
```go
func (s *MyService) Transfer(ctx context.Context, ... OtherParams) error {
	newCtx, span := s.tracer.Start(ctx, "coin_transfer",
		tracing.WithAttributes(tracing.String(currencyLabel, "CHF")),
		trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()
	...

	span.AddEvent("start_validation", tracing.WithAttributes(tracing.Int(validatorTypeLabel, 3)))
	err := s.otherService.ValidateTransfer(newCtx, ...)
	span.AddEvent("end_validation")
	
	...
	span.AddAttributes(tracing.Bool(successLabel, err == nil))
	return err
}
```

##### Add traces/spans in a View
In views, you can add events to the existing context as shown above. In most (or all) of the cases you will not need to create your own span, but use the existing one. 

If you though want to create one, since views do not have a proper dependency injection mechanism and creating a new tracer would be inefficient for each view call, you can use the following methods of `view.Context`:
```go
type Context interface {
	StartSpanFrom(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span)
	StartSpan(name string, opts ...trace.SpanStartOption) trace.Span
}
```
In most of the cases `StartSpan` is enough, as it uses the existing span from the view context. However, there may be cases where you want to create a child context, e.g. when you receive a message from a session:
```go
	ch := session.Receive()
	var payload []byte
	var rcvCtx context2.Context
	var rcvSpan trace.Span
	select {
	case msg := <-ch:
		payload = msg.Payload
		rcvCtx, rcvSpan = context.StartSpanFrom(msg.Ctx, "receive_response", trace.WithSpanKind(trace.SpanKindServer))
		defer rcvSpan.End()
		rcvSpan.AddLink(trace.Link{SpanContext: span.SpanContext()})
	case <-time.After(5 * time.Second):
		return nil, errors.New("time out reached")
	}
```
Note that you can also link two spans for better visualization on the UI.

### Visualization

Reading single spans from the console or a file is cumbersome and this is why we use tools that:
* collect the spans
* combine the spans by their metadata (span context) and compose traces
* allow querying of traces based on their name, labels, time, etc.
* visualize the traces on a UI

Although these components are separate and have distinct responsibilities, there is a docker image that combines all of them: `jaegertracing/all-in-one:latest`.
The `jaeger_collector` sub-image listens on a port for new spans. The application exports (sends) the spans to that endpoint (defined in `fsc.tracing.optl.address`).
The `jaeger_ui` serves an application with querying and trace-visualization capabilities. 
