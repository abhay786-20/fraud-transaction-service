package main

import (
	"context"
	"errors"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"go.uber.org/zap"

	"github.com/abhay786-20/fraud-transaction-service/internal/authclient"
	"github.com/abhay786-20/fraud-transaction-service/internal/config"
	"github.com/abhay786-20/fraud-transaction-service/internal/db"
	"github.com/abhay786-20/fraud-transaction-service/internal/handler"
	"github.com/abhay786-20/fraud-transaction-service/internal/repository"
	"github.com/abhay786-20/fraud-transaction-service/internal/router"
	"github.com/abhay786-20/fraud-transaction-service/internal/service"
	"github.com/abhay786-20/fraud-transaction-service/internal/worker"
	"github.com/abhay786-20/fraud-transaction-service/pkg/constants"
	"github.com/abhay786-20/fraud-transaction-service/pkg/env"
	"github.com/abhay786-20/fraud-transaction-service/pkg/kafka"
	"github.com/abhay786-20/fraud-transaction-service/pkg/logger"
)

func main() {
	_ = godotenv.Load()

	appEnv := env.GetString(constants.EnvAppEnv, constants.EnvValueDevelopment)

	log, err := logger.New("fraud-transaction-service", appEnv)
	if err != nil {
		panic(err)
	}
	defer log.Sync()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal("failed to load config", zap.Error(err))
	}

	pg, err := db.Connect(cfg.Database)
	if err != nil {
		log.Fatal("failed to connect to postgres", zap.Error(err))
	}
	defer pg.Close()
	log.Info("connected to postgres", zap.String("host", cfg.Database.Host), zap.String("db", cfg.Database.Name))

	if cfg.Env == constants.EnvValueProduction {
		gin.SetMode(gin.ReleaseMode)
	}

	// Wiring, bottom-up: repository → service → handler → router.
	txnRepo := repository.NewTransactionRepository(pg, log)
	authClient := authclient.New(cfg.Auth.ServiceBaseURL, cfg.Auth.ServiceAPIKey)
	txnService := service.NewTransactionService(txnRepo, authClient, log)
	txnHandler := handler.NewTransactionHandler(txnService)
	r := router.New(pg, cfg.Auth.JWTSecret, txnHandler)

	// Outbox worker: separate wiring path from the HTTP request path — it
	// reads outbox_events directly, publishes to Kafka, marks them done.
	outboxRepo := repository.NewOutboxRepository(pg, log)
	producer := kafka.NewProducer(cfg.Kafka.Brokers, cfg.Kafka.Topic)
	defer producer.Close()
	outboxWorker := worker.NewOutboxWorker(outboxRepo, producer, log)

	// ctx is cancelled the moment the OS sends SIGINT (Ctrl+C) or SIGTERM
	// (what Docker/Kubernetes send on a normal stop) — this is what lets
	// the worker's `for { select { case <-ctx.Done(): ... } }` loop know
	// to stop, instead of being killed mid-publish.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go outboxWorker.Run(ctx)

	// A plain http.Server (not Gin's router.Run(), which blocks forever
	// with no way to stop cleanly) — Run() has no equivalent of Shutdown().
	addr := cfg.Server.Host + ":" + cfg.Server.Port
	srv := &http.Server{Addr: addr, Handler: r}

	go func() {
		log.Info("starting server", zap.String("addr", addr), zap.String("env", cfg.Env))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal("server failed", zap.Error(err))
		}
	}()

	<-ctx.Done() // blocks here until SIGINT/SIGTERM arrives
	log.Info("shutdown signal received, shutting down gracefully")

	// A separate, bounded context for shutdown itself — gives in-flight
	// requests up to 10s to finish before forcing the connection closed,
	// rather than cutting them off instantly.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed", zap.Error(err))
	}

	log.Info("shutdown complete")
}
