package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"go.uber.org/zap"

	"github.com/abhay786-20/fraud-transaction-service/internal/config"
	"github.com/abhay786-20/fraud-transaction-service/internal/db"
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

	router := gin.Default()

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	router.GET("/ready", func(c *gin.Context) {
		if err := pg.PingContext(c.Request.Context()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready", "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})

	addr := cfg.Server.Host + ":" + cfg.Server.Port
	log.Info("starting server", zap.String("addr", addr), zap.String("env", cfg.Env))
	router.Run(addr)
}
