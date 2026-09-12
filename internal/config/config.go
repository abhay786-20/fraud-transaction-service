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
	}

	return cfg, nil
}
