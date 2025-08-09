/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"errors"
	"io"
	"net/http"
	"runtime"
	"strings"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric-lib-go/common/flogging/floggingtest"
	"github.com/hyperledger/fabric-lib-go/common/flogging/httpadmin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type logger struct {
	fabricLogger
	otelLogger
}

type (
	Config struct {
		// Format is the log record format specifier for the Logging instance. If the
		// spec is the string "json", log records will be formatted as JSON. Any
		// other string will be provided to the FormatEncoder. Please see
		// fabenc.ParseFormat for details on the supported verbs.
		//
		// If Format is not provided, a default format that provides basic information will
		// be used.
		Format string
		// LogSpec determines the log levels that are enabled for the logging system. The
		// spec must be in a format that can be processed by ActivateSpec.
		//
		// If LogSpec is not provided, loggers will be enabled at the INFO level.
		LogSpec string
		// Writer is the sink for encoded and formatted log records.
		//
		// If a Writer is not provided, os.Stderr will be used as the log sink.
		Writer io.Writer

		// OtelSanitize when set to true ensures that the strings representing events are first sanitized.
		// Sanitization means that any non-printable character is removed.
		// This protects the traces from undesired behaviours like missing tracing.
		OtelSanitize bool
	}
	Recorder = floggingtest.Recorder
	Option   = floggingtest.Option
)

// Logger provides logging API
type Logger interface {
	fabricLogger
	otelLogger

	With(args ...interface{}) Logger
	Named(name string) Logger
}

type fabricLogger interface {
	Debug(args ...interface{})
	Debugf(format string, args ...interface{})
	Error(args ...interface{})
	Errorf(format string, args ...interface{})
	Fatal(args ...interface{})
	Fatalf(format string, args ...interface{})
	Info(args ...interface{})
	Infof(format string, args ...interface{})
	Panic(args ...interface{})
	Panicf(format string, args ...interface{})
	Warn(args ...interface{})
	Warnf(format string, args ...interface{})
	IsEnabledFor(level zapcore.Level) bool
	Warnw(format string, args ...interface{})
	Warningf(format string, args ...interface{})
	Errorw(format string, args ...interface{})
	Zap() *zap.Logger
}

var (
	config = Config{}
)

func Named(loggerName string) Option {
	return func(r *floggingtest.RecordingCore, l *zap.Logger) *zap.Logger {
		return l.Named(loggerName)
	}
}

func MustGetLogger(params ...string) Logger {
	return utils.MustGet(GetLogger(params...))
}

func GetLogger(params ...string) (Logger, error) {
	return GetLoggerWithReplacements(map[string]string{"github.com.hyperledger-labs.fabric-smart-client.platform": "fsc"}, params)
}

func (l *logger) Named(name string) Logger {
	return newLogger(l.Zap().Named(name))
}

func (l *logger) With(args ...interface{}) Logger {
	return newLogger(l.Zap().Sugar().With(args...).Desugar())
}

func GetLoggerWithReplacements(replacements map[string]string, params []string) (Logger, error) {
	fullPkgName, err := GetPackageName()
	if err != nil {
		return nil, err
	}
	name := loggerName(fullPkgName, replacements, params...)
	return newLogger(flogging.Global.ZapLogger(name)), nil
}

func newLogger(zapLogger *zap.Logger) *logger {
	return &logger{
		fabricLogger: flogging.NewFabricLogger(zapLogger),
		otelLogger:   NewOtelLogger(zapLogger),
	}
}

func GetPackageName() (string, error) {
	pc, _, _, ok := runtime.Caller(4)
	if !ok {
		return "", errors.New("failed to get caller package name")
	}
	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return "", errors.New("failed to get caller package name")
	}
	fullFuncName := fn.Name()
	lastSlash := strings.LastIndex(fullFuncName, "/")
	dotAfterSlash := strings.Index(fullFuncName[lastSlash:], ".")
	return fullFuncName[:lastSlash+dotAfterSlash], nil
}

func loggerName(fullPkgName string, replacements map[string]string, params ...string) string {
	nameParts := append(strings.Split(fullPkgName, "/"), params...)
	name := strings.Join(nameParts, ".")

	for old, newVal := range replacements {
		name = strings.ReplaceAll(name, old, newVal)
	}
	return name
}

func NewTestLogger(tb testing.TB, options ...Option) (Logger, *Recorder) {
	l, r := floggingtest.NewTestLogger(tb, options...)
	return &logger{fabricLogger: l}, r
}

func NewSpecHandler() http.Handler {
	return httpadmin.NewSpecHandler()
}

func Init(c Config) {
	flogging.Init(flogging.Config{
		Format:  c.Format,
		LogSpec: c.LogSpec,
		Writer:  c.Writer,
	})

	// set local configurations
	config.OtelSanitize = c.OtelSanitize
}
