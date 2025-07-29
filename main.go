package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	docs "mq-transfer-go/api/docs"
	"mq-transfer-go/api/handlers"
	"mq-transfer-go/internal/otelutils"
	"mq-transfer-go/transferstore"
	dynamostore "mq-transfer-go/transferstore/dynamo"
	sqlitestore "mq-transfer-go/transferstore/sqlite"
)

var logFatalf = log.Fatalf
var serverAddr = ":8080"
var serverShutdown = func(srv *http.Server, ctx context.Context) error {
	return srv.Shutdown(ctx)
}
var otelInit = otelutils.InitOTel

// @title MQ Transfer API
// @version 1.0
// @description API para transferência de mensagens entre filas IBM MQ
// @termsOfService http://swagger.io/terms/
// @contact.name API Support
// @contact.url http://www.example.com/support
// @contact.email support@example.com
// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html
// @host localhost:8080
// @BasePath /
// @schemes http
func main() {
	otelConfig := otelutils.OTelConfig{
		ServiceName:    "mq-transfer-service",
		ServiceVersion: "1.0.0",
		Environment:    os.Getenv("ENV"),
		OTLPEndpoint:   os.Getenv("OTLP_ENDPOINT"),
	}
	if otelConfig.Environment == "" {
		otelConfig.Environment = "development"
	}
	_, err := otelInit(otelConfig)
	if err != nil {
		log.Printf("Aviso: Falha ao inicializar OpenTelemetry: %v. Continuando sem telemetria.", err)
	}

	var store transferstore.TransferStore
	if os.Getenv("USE_DYNAMO") == "true" {
		cfg, err := config.LoadDefaultConfig(context.Background())
		if err != nil {
			logFatalf("falha ao carregar config AWS: %v", err)
		}
		store = dynamostore.NewDynamoStore(dynamodb.NewFromConfig(cfg), "transfer_requests")
	} else {
		var err error
		path := os.Getenv("DB_PATH")
		if path == "" {
			path = "/data/transfer.db"
		}
		store, err = sqlitestore.NewSQLiteStore(path)
		if err != nil {
			logFatalf("falha ao inicializar sqlite: %v", err)
		}
	}
	handler := handlers.NewTransferHandler(store)
	if hs, ok := store.(handlers.Pinger); ok {
		handlers.HealthStore = hs
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	r := gin.Default()
	// Use empty host so swagger calls the same host that served the docs
	docs.OpenAPIInfo.Host = ""
	v1 := r.Group("/api/v1")
	{
		v1.POST("/transfer", handler.StartTransfer)
		v1.GET("/transfer/:requestId", handler.GetTransferStatus)
		v1.GET("/transfers", handler.ListTransfers)
		v1.POST("/transfer/:requestId/cancel", handler.CancelTransfer)
		v1.GET("/health", handlers.HealthCheck)
	}
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	srv := &http.Server{
		Addr:    serverAddr,
		Handler: r,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logFatalf("Falha ao iniciar o servidor: %v", err)
		}
	}()

	log.Println("Servidor iniciado na porta", serverAddr)
	log.Println("Documentação Swagger disponível em: /swagger/index.html")

	<-ctx.Done()
	log.Println("Desligando servidor...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := serverShutdown(srv, shutdownCtx); err != nil {
		logFatalf("Erro ao desligar o servidor: %v", err)
	}

	otelutils.Shutdown(shutdownCtx)

	log.Println("Servidor encerrado com sucesso")
}
