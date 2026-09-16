/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"net/http"
	"runtime"
	"strings"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric-lib-go/common/flogging/httpadmin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// Logger provides logging API
type Logger interface {
	fabricLogger
	otelLogger

	With(args ...any) Logger
	Named(name string) Logger
}

type fabricLogger interface {
	Debug(args ...any)
	Debugf(format string, args ...any)
	Error(args ...any)
	Errorf(format string, args ...any)
	Fatal(args ...any)
	Fatalf(format string, args ...any)
	Info(args ...any)
	Infof(format string, args ...any)
	Panic(args ...any)
	Panicf(format string, args ...any)
	Warn(args ...any)
	Warnf(format string, args ...any)
	IsEnabledFor(level zapcore.Level) bool
	Warnw(format string, args ...any)
	Warningf(format string, args ...any)
	Errorw(format string, args ...any)
	Zap() *zap.Logger
}

type logger struct {
	fabricLogger
	otelLogger
}

func newLogger(zapLogger *zap.Logger) *logger {
	return &logger{
		fabricLogger: flogging.NewFabricLogger(zapLogger),
		// One frame to skip: the ctxFieldLogger.*Context method that forwards to zap.
		otelLogger: NewOtelLogger(zapLogger.WithOptions(zap.AddCallerSkip(1))),
	}
}

func (l *logger) Named(name string) Logger {
	return newLogger(l.Zap().Named(name))
}

func (l *logger) With(args ...any) Logger {
	return newLogger(l.Zap().Sugar().With(args...).Desugar())
}

// MustGetLogger returns the same logger as GetLogger, panicking instead of
// returning an error. This follows the regexp.MustCompile convention: it is
// meant for the near-universal call pattern var logger =
// logging.MustGetLogger() at package scope, where there is no caller to
// propagate an error to. GetLogger only fails when GetPackageName cannot
// determine the caller's package (see its Godoc), a static invariant of the
// process rather than runtime input, so panicking on it is a programmer-error
// signal, not a crash on bad data.
func MustGetLogger(params ...string) Logger {
	l, err := GetLogger(params...)
	if err != nil {
		panic(err)
	}
	return l
}

// GetLogger returns a Logger named after the caller's package, with any
// registered Replacers applied and params appended to the name. It fails
// only if GetPackageName cannot resolve the caller.
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

// GetPackageName resolves the package path of the caller four stack frames
// above its own, an offset fixed to the GetLogger/GetLoggerWithReplacements
// chain that GetLogger and MustGetLogger call it through. It returns an
// error rather than panicking if that frame cannot be found or resolved to a
// function, since callers other than that fixed chain (direct callers, or
// tests calling this package's exported functions at a different depth) can
// legitimately hit it.
func GetPackageName() (string, error) {
	pc, _, _, ok := runtime.Caller(4)
	if !ok {
		return "", errors.New("failed to get caller package name")
	}
	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return "", errors.New("failed to get caller package name")
	}
	// A function name from a module path looks like
	// github.com/org/repo/pkg.Func, so the package name is everything up to the
	// first dot after the last slash. Names without a slash (the standard
	// library's, for instance) do not have that shape, and neither does one
	// whose final segment carries no dot, so report those rather than slicing
	// with a negative index.
	fullFuncName := fn.Name()
	lastSlash := strings.LastIndex(fullFuncName, "/")
	if lastSlash < 0 {
		return "", errors.Errorf("caller package name has no path separator: %s", fullFuncName)
	}
	dotAfterSlash := strings.Index(fullFuncName[lastSlash:], ".")
	if dotAfterSlash < 0 {
		return "", errors.Errorf("caller package name has no function separator: %s", fullFuncName)
	}

	return fullFuncName[:lastSlash+dotAfterSlash], nil
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
