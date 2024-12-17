/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"testing"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"github.com/hyperledger/fabric-lib-go/common/flogging/floggingtest"
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
	With(args ...string) Logger
	Zap() *zap.Logger
}

type Recorder = floggingtest.Recorder

type Option = floggingtest.Option

func MustGetLogger(loggerName string) Logger {
	return &logger{FabricLogger: flogging.MustGetLogger(loggerName)}
}

func NewTestLogger(tb testing.TB, options ...Option) (Logger, *Recorder) {
	l, r := floggingtest.NewTestLogger(tb, options...)
	return &logger{FabricLogger: l}, r
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

func (l *logger) With(args ...string) Logger {
	return &logger{FabricLogger: l.FabricLogger.With(args)}
}
