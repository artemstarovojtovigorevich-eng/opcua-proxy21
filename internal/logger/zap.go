package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Logger struct {
	sugar *zap.SugaredLogger
	core  *zap.Logger
}

func New(level, encoding string) (*Logger, error) {
	var zapLevel zapcore.Level
	switch level {
	case "debug":
		zapLevel = zapcore.DebugLevel
	case "info":
		zapLevel = zapcore.InfoLevel
	case "warn":
		zapLevel = zapcore.WarnLevel
	case "error":
		zapLevel = zapcore.ErrorLevel
	default:
		zapLevel = zapcore.InfoLevel
	}

	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	var encoder zapcore.Encoder
	if encoding == "json" {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	} else {
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	}

	core := zapcore.NewCore(
		encoder,
		zapcore.AddSync(os.Stdout),
		zapLevel,
	)

	logger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
	sugar := logger.Sugar()

	return &Logger{sugar: sugar, core: logger}, nil
}

func (l *Logger) Debug(msg string, args ...interface{}) {
	l.sugar.Debugw(msg, args...)
}

func (l *Logger) Info(msg string, args ...interface{}) {
	l.sugar.Infow(msg, args...)
}

func (l *Logger) Warn(msg string, args ...interface{}) {
	l.sugar.Warnw(msg, args...)
}

func (l *Logger) Error(msg string, args ...interface{}) {
	l.sugar.Errorw(msg, args...)
}

func (l *Logger) Fatal(msg string, args ...interface{}) {
	l.sugar.Fatalw(msg, args...)
	os.Exit(1)
}

func (l *Logger) Sync() {
	_ = l.core.Sync()
}
