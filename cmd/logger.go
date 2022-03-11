package cmd

import (
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	FlagLogLevel  = LogLevel{zap.InfoLevel}
	FlagLogFormat = FlagSetCommon.String("log-format", "console", "Log format (console, json)")
)

func init() {
	FlagSetCommon.Var(&FlagLogLevel, "log-level", "Log level")
}

func GetLogger() *zap.Logger {
	var config zap.Config
	if *FlagLogFormat == "json" {
		config = zap.NewProductionConfig()
	} else {
		config = zap.NewDevelopmentConfig()
		config.DisableStacktrace = true
	}
	config.DisableCaller = true
	config.Level.SetLevel(FlagLogLevel.Level)
	logger, err := config.Build()
	cobra.CheckErr(err)
	return logger
}

// LogLevel is required to use zap level as a pflag.
type LogLevel struct{ zapcore.Level }

func (LogLevel) Type() string {
	return "string"
}
