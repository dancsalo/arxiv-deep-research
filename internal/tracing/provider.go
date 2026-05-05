package tracing

import (
	"context"
	"encoding/base64"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

type Config struct {
	Endpoint    string
	PublicKey   string
	SecretKey   string
	ServiceName string
}

func (c Config) Enabled() bool {
	return c.Endpoint != "" && c.PublicKey != "" && c.SecretKey != ""
}

func InitTracer(ctx context.Context, cfg Config) (*sdktrace.TracerProvider, func(context.Context)) {
	if !cfg.Enabled() {
		noop := sdktrace.NewTracerProvider()
		return noop, func(context.Context) {}
	}

	serviceName := cfg.ServiceName
	if serviceName == "" {
		serviceName = "arxiv-deep-research"
	}

	auth := base64.StdEncoding.EncodeToString([]byte(cfg.PublicKey + ":" + cfg.SecretKey))

	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(cfg.Endpoint),
		otlptracehttp.WithHeaders(map[string]string{
			"Authorization": "Basic " + auth,
		}),
	)
	if err != nil {
		otel.Handle(err)
		noop := sdktrace.NewTracerProvider()
		return noop, func(context.Context) {}
	}

	res, _ := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
		),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)

	shutdown := func(ctx context.Context) {
		_ = tp.Shutdown(ctx)
	}
	return tp, shutdown
}
