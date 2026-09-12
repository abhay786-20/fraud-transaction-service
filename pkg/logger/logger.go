package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/abhay786-20/fraud-transaction-service/pkg/constants"
)

func New(serviceName, env string) (*zap.Logger, error) {
	stacktraceAt := zap.AddStacktrace(zapcore.ErrorLevel)

	var base *zap.Logger
	var err error

	if env == constants.EnvValueProduction {
		base, err = zap.NewProduction(stacktraceAt)
	} else {
		base, err = zap.NewDevelopment(stacktraceAt)
	}
	if err != nil {
		return nil, err
	}

	return base.With(zap.String("service", serviceName)), nil
}
