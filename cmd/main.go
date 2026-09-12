package main

import (
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"go.uber.org/zap"

	"github.com/abhay786-20/fraud-transaction-service/internal/config"
	"github.com/abhay786-20/fraud-transaction-service/internal/db"
	"github.com/abhay786-20/fraud-transaction-service/internal/handler"
	"github.com/abhay786-20/fraud-transaction-service/internal/repository"
	"github.com/abhay786-20/fraud-transaction-service/internal/router"
	"github.com/abhay786-20/fraud-transaction-service/internal/service"
	"github.com/abhay786-20/fraud-transaction-service/pkg/constants"
	"github.com/abhay786-20/fraud-transaction-service/pkg/env"
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
	txnService := service.NewTransactionService(txnRepo, log)
	txnHandler := handler.NewTransactionHandler(txnService)
	r := router.New(pg, cfg.Auth.JWTSecret, txnHandler)

	addr := cfg.Server.Host + ":" + cfg.Server.Port
	log.Info("starting server", zap.String("addr", addr), zap.String("env", cfg.Env))
	r.Run(addr)
}
