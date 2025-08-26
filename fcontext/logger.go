// Defined logger
package fcontext

import (
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
	WithPrefix(prefix string) Logger
}

type ZeroLogger struct {
	logger zerolog.Logger
}

func newZeroLogger(prefix string) *ZeroLogger {
	w := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339Nano}
	z := log.Output(w)
	if prefix != "" {
		z = z.With().Str("service", prefix).Logger()
	}
	return &ZeroLogger{logger: z}
}

func (l *ZeroLogger) Debug(msg string, args ...any) { l.logger.Debug().Msgf(msg, args...) }
func (l *ZeroLogger) Info(msg string, args ...any)  { l.logger.Info().Msgf(msg, args...) }
func (l *ZeroLogger) Warn(msg string, args ...any)  { l.logger.Warn().Msgf(msg, args...) }
func (l *ZeroLogger) Error(msg string, args ...any) { l.logger.Error().Msgf(msg, args...) }

func (l *ZeroLogger) WithPrefix(prefix string) Logger {
	return &ZeroLogger{logger: l.logger.With().Str("prefix", prefix).Logger()}
}
