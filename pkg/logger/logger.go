package logger

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New creates a production-ready structured logger
func New(level, environment string) (*zap.Logger, error) {
	var cfg zap.Config

	if environment == "production" {
		cfg = zap.NewProductionConfig()
		cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	} else {
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	parsed, err := zapcore.ParseLevel(level)
	if err != nil {
		return nil, fmt.Errorf("invalid log level %q: %w", level, err)
	}
	cfg.Level = zap.NewAtomicLevelAt(parsed)

	return cfg.Build(
		zap.AddCaller(),
		zap.AddStacktrace(zap.ErrorLevel),
	)
}
