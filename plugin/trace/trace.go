// Package trace implements OpenTracing-based tracing
package trace

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/metrics"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/rcode"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/opentracer"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	// Plugin the trace package.
	_ "github.com/coredns/coredns/plugin/pkg/trace"
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	ot "github.com/opentracing/opentracing-go"
	zipkin "github.com/openzipkin-contrib/zipkin-go-opentracing"
)

const (
	tagClient = "coredns.client"
	tagName   = "coredns.name"
	tagProto  = "coredns.proto"
	tagRcode  = "coredns.rcode"
	tagServer = "coredns.server"
	tagType   = "coredns.type"
)

type trace struct {
	Next            plugin.Handler
	Endpoint        string
	EndpointType    string
	tracer          ot.Tracer
	serviceEndpoint string
	serviceName     string
	clientServer    bool
	every           uint64
	count           uint64
	Once            sync.Once
}

func (t *trace) Tracer() ot.Tracer {
	return t.tracer
}

// OnStartup sets up the tracer
func (t *trace) OnStartup() error {
	var err error
	t.Once.Do(func() {
		switch t.EndpointType {
		case "zipkin":
			err = t.setupZipkin()
		case "datadog":
			tracer := opentracer.New(tracer.WithAgentAddr(t.Endpoint), tracer.WithServiceName(t.serviceName), tracer.WithDebugMode(true))
			t.tracer = tracer
		default:
			err = fmt.Errorf("unknown endpoint type: %s", t.EndpointType)
		}
	})
	return err
}

func (t *trace) setupZipkin() error {

	collector, err := zipkin.NewHTTPCollector(t.Endpoint)
	if err != nil {
		return err
	}

	recorder := zipkin.NewRecorder(collector, false, t.serviceEndpoint, t.serviceName)
	t.tracer, err = zipkin.NewTracer(recorder, zipkin.ClientServerSameSpan(t.clientServer))

	return err
}

// Name implements the Handler interface.
func (t *trace) Name() string { return "trace" }

// ServeDNS implements the plugin.Handle interface.
func (t *trace) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	trace := false
	if t.every > 0 {
		queryNr := atomic.AddUint64(&t.count, 1)

		if queryNr%t.every == 0 {
			trace = true
		}
	}
	span := ot.SpanFromContext(ctx)
	if !trace || span != nil {
		return plugin.NextOrFailure(t.Name(), t.Next, ctx, w, r)
	}

	req := request.Request{W: w, Req: r}
	span = t.Tracer().StartSpan("servedns")
	defer span.Finish()

	rw := dnstest.NewRecorder(w)
	ctx = ot.ContextWithSpan(ctx, span)
	status, err := plugin.NextOrFailure(t.Name(), t.Next, ctx, rw, r)

	span.SetTag(tagClient, req.IP())
	span.SetTag(tagName, req.Name())
	span.SetTag(tagProto, req.Proto())
	span.SetTag(tagRcode, rcode.ToString(rw.Rcode))
	span.SetTag(tagServer, metrics.WithServer(ctx))
	span.SetTag(tagType, req.Type())
	span.SetTag("_dd1.sr.eausr",1)
	span.SetTag("_dd1.sr.rapre",float64(1/t.every))

	return status, err
}
