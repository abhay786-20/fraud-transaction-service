package constants

const (
	EnvAppEnv = "TRANSACTION_ENV"

	EnvServerHost = "TRANSACTION_SERVER_HOST"
	EnvServerPort = "TRANSACTION_SERVER_PORT"

	EnvDBHost     = "TRANSACTION_DB_HOST"
	EnvDBPort     = "TRANSACTION_DB_PORT"
	EnvDBUser     = "TRANSACTION_DB_USER"
	EnvDBPassword = "TRANSACTION_DB_PASSWORD"
	EnvDBName     = "TRANSACTION_DB_NAME"

	EnvDBMaxOpenConns   = "TRANSACTION_DB_MAX_OPEN_CONNS"
	EnvDBMaxIdleConns   = "TRANSACTION_DB_MAX_IDLE_CONNS"
	EnvDBMaxLifetimeMin = "TRANSACTION_DB_MAX_LIFETIME_MIN"
)

// RequiredEnvKeys lists every env var this service cannot start without.
var RequiredEnvKeys = []string{
	EnvDBUser,
	EnvDBPassword,
	EnvDBName,
}

// Accepted values for TRANSACTION_ENV — the app's own convention, not an
// OS contract, but shared across multiple files, so it lives here too.
const (
	EnvValueDevelopment = "development"
	EnvValueProduction  = "production"
)
