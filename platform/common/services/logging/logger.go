/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"errors"
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

type (
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

type logger struct {
	fabricLogger
	otelLogger
}

func newLogger(zapLogger *zap.Logger) *logger {
	return &logger{
		fabricLogger: flogging.NewFabricLogger(zapLogger),
		otelLogger:   NewOtelLogger(zapLogger.WithOptions(zap.AddCallerSkip(1))),
	}
}

func (l *logger) Named(name string) Logger {
	return newLogger(l.Zap().Named(name))
}

func (l *logger) With(args ...interface{}) Logger {
	return newLogger(l.Zap().Sugar().With(args...).Desugar())
}

func Named(loggerName string) Option {
	return func(r *floggingtest.RecordingCore, l *zap.Logger) *zap.Logger {
		return l.Named(loggerName)
	}
}

func MustGetLogger(params ...string) Logger {
	return utils.MustGet(GetLogger(params...))
}

func GetLogger(params ...string) (Logger, error) {
	return GetLoggerWithReplacements(Replacers(), params)
}

func GetLoggerWithReplacements(replacements map[string]string, params []string) (Logger, error) {
	fullPkgName, err := GetPackageName()
	if err != nil {
		return nil, err
	}
	name := loggerName(fullPkgName, replacements, params...)
	return newLogger(flogging.Global.ZapLogger(name)), nil
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

func NewTestLogger(tb testing.TB, options ...Option) (Logger, *Recorder) {
	l, r := floggingtest.NewTestLogger(tb, options...)
	return &logger{fabricLogger: l}, r
}

func NewSpecHandler() http.Handler {
	return httpadmin.NewSpecHandler()
}

func loggerName(fullPkgName string, replacements map[string]string, params ...string) string {
	nameParts := append(strings.Split(fullPkgName, "/"), params...)
	name := strings.Join(nameParts, ".")

	for old, newVal := range replacements {
		name = strings.ReplaceAll(name, old, newVal)
	}
	return name
}
