package otelutils

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/aggregation"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

// MQMetrics contém os medidores para métricas relacionadas à transferência MQ
type MQMetrics struct {
	MessagesTransferred metric.Int64Counter
	BytesTransferred    metric.Int64Counter
	TransferDuration    metric.Float64Histogram
	CommitCounter       metric.Int64Counter
	ErrorCounter        metric.Int64Counter
	Meter               metric.Meter
}

// OTelConfig contém a configuração para o OpenTelemetry
type OTelConfig struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	OTLPEndpoint   string
}

var (
	mp      *sdkmetric.MeterProvider
	metrics *MQMetrics
)

// InitOTel inicializa o OpenTelemetry SDK
func InitOTel(config OTelConfig) (*MQMetrics, error) {
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(config.ServiceName),
			semconv.ServiceVersion(config.ServiceVersion),
			semconv.DeploymentEnvironment(config.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("falha ao criar recurso: %v", err)
	}

	ctx := context.Background()
	exp, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(config.OTLPEndpoint),
		otlpmetricgrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("falha ao criar exportador OTLP: %v", err)
	}

	mp = sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(
			sdkmetric.NewPeriodicReader(
				exp,
				sdkmetric.WithInterval(10*time.Second),
				sdkmetric.WithTimeout(5*time.Second),
			),
		),
		sdkmetric.WithView(
			sdkmetric.NewView(
				sdkmetric.Instrument{
					Name: "mq_transfer_duration",
					Kind: sdkmetric.InstrumentKindHistogram,
				},
				sdkmetric.Stream{
					Aggregation: aggregation.ExplicitBucketHistogram{
						Boundaries: []float64{
							1, 5, 10, 25, 50, 75, 100, 250, 500, 750, 1000, 2500, 5000, 7500, 10000,
						},
					},
				},
			),
		),
	)

	otel.SetMeterProvider(mp)

	meter := mp.Meter("mq-transfer-service")

	messagesCounter, err := meter.Int64Counter(
		"mq_messages_transferred",
		metric.WithDescription("Número total de mensagens transferidas"),
		metric.WithUnit("{messages}"),
	)
	if err != nil {
		return nil, fmt.Errorf("falha ao criar contador de mensagens: %v", err)
	}

	bytesCounter, err := meter.Int64Counter(
		"mq_bytes_transferred",
		metric.WithDescription("Número total de bytes transferidos"),
		metric.WithUnit("By"),
	)
	if err != nil {
		return nil, fmt.Errorf("falha ao criar contador de bytes: %v", err)
	}

	durationHistogram, err := meter.Float64Histogram(
		"mq_transfer_duration",
		metric.WithDescription("Duração da transferência de mensagens"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, fmt.Errorf("falha ao criar histograma de duração: %v", err)
	}

	commitCounter, err := meter.Int64Counter(
		"mq_commits",
		metric.WithDescription("Número total de commits realizados"),
		metric.WithUnit("{commits}"),
	)
	if err != nil {
		return nil, fmt.Errorf("falha ao criar contador de commits: %v", err)
	}

	errorCounter, err := meter.Int64Counter(
		"mq_errors",
		metric.WithDescription("Número total de erros ocorridos"),
		metric.WithUnit("{errors}"),
	)
	if err != nil {
		return nil, fmt.Errorf("falha ao criar contador de erros: %v", err)
	}

	metrics = &MQMetrics{
		MessagesTransferred: messagesCounter,
		BytesTransferred:    bytesCounter,
		TransferDuration:    durationHistogram,
		CommitCounter:       commitCounter,
		ErrorCounter:        errorCounter,
		Meter:               meter,
	}

	log.Println("OpenTelemetry inicializado com sucesso")
	return metrics, nil
}

// Shutdown encerra o provedor de métricas
func Shutdown(ctx context.Context) {
	if mp != nil {
		if err := mp.Shutdown(ctx); err != nil {
			log.Printf("Erro ao encerrar o provedor de métricas: %v", err)
		}
	}
}

// GetMetrics retorna as métricas configuradas
func GetMetrics() *MQMetrics {
	return metrics
}
