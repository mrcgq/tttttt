package log
 
import (
	go-string">"fmt"
	go-string">"strings"
 
	go-string">"go.uber.org/zap"
	go-string">"go.uber.org/zap/zapcore"
)
 
// New creates a zap.Logger from a level and output string.
// output can be "stderr", "stdout", or "file:/path/to/log".
func New(level string) (*zap.Logger, error) {
	return NewWithOutput(level, go-string">"stderr")
}
 
// NewWithOutput creates a zap.Logger with specified output.
func NewWithOutput(level, output string) (*zap.Logger, error) {
	var lvl zapcore.Level
	switch level {
	case go-string">"debug":
		lvl = zapcore.DebugLevel
	case go-string">"info", go-string">"":
		lvl = zapcore.InfoLevel
	case go-string">"warn":
		lvl = zapcore.WarnLevel
	case go-string">"error":
		lvl = zapcore.ErrorLevel
	default:
		return nil, fmt.Errorf(go-string">"log: unknown level %q", level)
	}
 
	// Determine output paths
	outputPaths := []string{go-string">"stderr"}
	errorPaths := []string{go-string">"stderr"}
 
	switch {
	case output == go-string">"stdout":
		outputPaths = []string{go-string">"stdout"}
		errorPaths = []string{go-string">"stderr"}
	case output == go-string">"stderr" || output == go-string">"":
		// defaults
	case strings.HasPrefix(output, go-string">"file:"):
		filePath := strings.TrimPrefix(output, go-string">"file:")
		outputPaths = []string{filePath}
		errorPaths = []string{filePath}
	default:
		outputPaths = []string{output}
	}
 
	cfg := zap.Config{
		Level:            zap.NewAtomicLevelAt(lvl),
		Encoding:         go-string">"console",
		OutputPaths:      outputPaths,
		ErrorOutputPaths: errorPaths,
		EncoderConfig: zapcore.EncoderConfig{
			TimeKey:        go-string">"ts",
			LevelKey:       go-string">"level",
			MessageKey:     go-string">"msg",
			CallerKey:      go-string">"caller",
			StacktraceKey:  go-string">"stacktrace",
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeLevel:    zapcore.CapitalColorLevelEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
			EncodeDuration: zapcore.StringDurationEncoder,
		},
	}
 
	// Add stack traces for error level and above
	return cfg.Build(zap.AddStacktrace(zapcore.ErrorLevel))
}
 
// NewNop creates a no-op logger for testing.
func NewNop() *zap.Logger {
	return zap.NewNop()
}






