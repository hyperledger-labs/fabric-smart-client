/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric-lib-go/common/flogging/floggingtest"
	"github.com/hyperledger/fabric-lib-go/common/flogging/httpadmin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger provides logging API
type Logger interface {
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
	Named(name string) Logger
	Warnw(format string, args ...interface{})
	Warningf(format string, args ...interface{})
	Errorw(format string, args ...interface{})
	With(args ...interface{}) Logger
	Zap() *zap.Logger
}

type Recorder = floggingtest.Recorder

type Option = floggingtest.Option

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

func GetLoggerWithReplacements(replacements map[string]string, params []string) (Logger, error) {
	fullPkgName, err := GetPackageName()
	if err != nil {
		return nil, err
	}
	return &logger{FabricLogger: flogging.MustGetLogger(loggerName(fullPkgName, replacements, params...))}, nil
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
	return &logger{FabricLogger: l}, r
}

func NewSpecHandler() http.Handler {
	return httpadmin.NewSpecHandler()
}

type Config = flogging.Config

func Init(c Config) {
	flogging.Init(c)
}

type logger struct {
	*flogging.FabricLogger
}

func (l *logger) Named(name string) Logger {
	return &logger{FabricLogger: l.FabricLogger.Named(name)}
}

func (l *logger) With(args ...interface{}) Logger {
	return &logger{FabricLogger: l.FabricLogger.With(args...)}
}

func Keys[K comparable, V any](m map[K]V) fmt.Stringer {
	return keys[K, V](m)
}

type keys[K comparable, V any] map[K]V

func (k keys[K, V]) String() string {
	return fmt.Sprintf(strings.Join(collections.Repeat("%v", len(k)), ", "), collections.Keys(k))
}

func Base64(b []byte) base64Enc {
	return b
}

type base64Enc []byte

func (b base64Enc) String() string {
	return base64.StdEncoding.EncodeToString(b)
}
