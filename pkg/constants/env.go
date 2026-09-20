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

	// Must hold the SAME VALUE as fraud-auth-service's AUTH_JWT_SECRET —
	// different env var name, shared secret, kept in sync manually. See
	// coding-plan.md §5 for why (shared-secret approach, deferred the
	// asymmetric-key upgrade).
	EnvJWTSecret = "TRANSACTION_JWT_SECRET"

	EnvKafkaBrokers = "TRANSACTION_KAFKA_BROKERS"
	EnvKafkaTopic   = "TRANSACTION_KAFKA_TOPIC"
	// EnvKafkaAlertsTopic must hold the SAME VALUE as fraud-engine-service's
	// KAFKA_ALERTS_TOPIC — this service publishes directly to it for
	// admin-triggered manual flags (see service.TransactionService.Flag),
	// the same topic/shape fraud-engine-service publishes to automatically.
	EnvKafkaAlertsTopic = "TRANSACTION_KAFKA_ALERTS_TOPIC"

	// For calling fraud-auth-service's /internal/* routes to verify a
	// receiver exists. EnvAuthServiceAPIKey must match fraud-auth-service's
	// AUTH_INTERNAL_API_KEY exactly — same shared-secret pattern as the JWT.
	EnvAuthServiceBaseURL = "TRANSACTION_AUTH_SERVICE_URL"
	EnvAuthServiceAPIKey  = "TRANSACTION_INTERNAL_API_KEY"
)

// RequiredEnvKeys lists every env var this service cannot start without.
var RequiredEnvKeys = []string{
	EnvDBUser,
	EnvDBPassword,
	EnvDBName,
	EnvJWTSecret,
	EnvAuthServiceAPIKey,
}

// Accepted values for TRANSACTION_ENV — the app's own convention, not an
// OS contract, but shared across multiple files, so it lives here too.
const (
	EnvValueDevelopment = "development"
	EnvValueProduction  = "production"
)
