package config

import (
	"os"

	"github.com/abhay786-20/fraud-transaction-service/pkg/constants"
	"github.com/abhay786-20/fraud-transaction-service/pkg/env"
)

type Config struct {
	Env      string
	Server   ServerConfig
	Database DatabaseConfig
	Auth     AuthConfig
	Kafka    KafkaConfig
}

type ServerConfig struct {
	Host string
	Port string
}

type DatabaseConfig struct {
	Host            string
	Port            string
	User            string
	Password        string
	Name            string
	MaxOpenConns    int
	MaxIdleConns    int
	MaxLifetimeMins int
}

// AuthConfig holds what this service needs both to VERIFY tokens issued
// by fraud-auth-service, and to CALL its internal API (service-to-service,
// via ServiceBaseURL/ServiceAPIKey) — never to issue tokens itself.
type AuthConfig struct {
	JWTSecret      string
	ServiceBaseURL string
	ServiceAPIKey  string
}

type KafkaConfig struct {
	Brokers []string
	Topic   string
}

func Load() (*Config, error) {
	if err := env.ValidateRequired(constants.RequiredEnvKeys); err != nil {
		return nil, err
	}

	cfg := &Config{
		Env: env.GetString(constants.EnvAppEnv, constants.EnvValueDevelopment),
		Server: ServerConfig{
			Host: env.GetString(constants.EnvServerHost, "0.0.0.0"),
			Port: env.GetString(constants.EnvServerPort, "8082"),
		},
		Database: DatabaseConfig{
			Host:            env.GetString(constants.EnvDBHost, "localhost"),
			Port:            env.GetString(constants.EnvDBPort, "5432"),
			User:            os.Getenv(constants.EnvDBUser),
			Password:        os.Getenv(constants.EnvDBPassword),
			Name:            os.Getenv(constants.EnvDBName),
			MaxOpenConns:    env.GetInt(constants.EnvDBMaxOpenConns, 25),
			MaxIdleConns:    env.GetInt(constants.EnvDBMaxIdleConns, 25),
			MaxLifetimeMins: env.GetInt(constants.EnvDBMaxLifetimeMin, 5),
		},
		Auth: AuthConfig{
			JWTSecret:      os.Getenv(constants.EnvJWTSecret),
			ServiceBaseURL: env.GetString(constants.EnvAuthServiceBaseURL, "http://localhost:8081"),
			ServiceAPIKey:  os.Getenv(constants.EnvAuthServiceAPIKey),
		},
		Kafka: KafkaConfig{
			Brokers: env.GetStringSlice(constants.EnvKafkaBrokers, []string{"localhost:9092"}),
			Topic:   env.GetString(constants.EnvKafkaTopic, "transactions"),
		},
	}

	return cfg, nil
}
