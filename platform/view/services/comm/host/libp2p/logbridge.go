/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package libp2p

import (
	"context"
	"log/slog"

	golog "github.com/ipfs/go-log/v2"
	"go.uber.org/zap/zapcore"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
)

// libp2pLoggerName is the logging.spec name of every libp2p line. The go-log
// subsystem is an attribute, not part of the name.
const libp2pLoggerName = "libp2p"

// subsystemAttr is the attribute that carries the go-log subsystem, such as basichost.
const subsystemAttr = "subsystem"

// routeLibp2pLogs sends go-log lines through FSC logging under [libp2pLoggerName].
// go-log filters by its own level before the core sees a line, so the filter is
// opened and logging.spec decides. "*" also sets the level of a subsystem logger
// created later, which SetAllLoggers does not.
func routeLibp2pLogs() {
	golog.SetPrimaryCore(&fscCore{h: logging.Handler(libp2pLoggerName)})
	if err := golog.SetLogLevel("*", "debug"); err != nil {
		logger.Errorf("libp2p: failed opening go-log levels: %s", err)
	}
}

// fscCore is a zap core that forwards go-log entries to an FSC slog handler.
type fscCore struct {
	h      slog.Handler
	fields []zapcore.Field
}

func (c *fscCore) Enabled(lvl zapcore.Level) bool {
	return c.h.Enabled(context.Background(), slogLevel(lvl))
}

func (c *fscCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(ent.Level) {
		return ce.AddCore(ent, c)
	}
	return ce
}

func (c *fscCore) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	level := slogLevel(ent.Level)
	if !c.h.Enabled(context.Background(), level) {
		return nil
	}
	r := slog.NewRecord(ent.Time, level, ent.Message, ent.Caller.PC)
	r.AddAttrs(slog.String(subsystemAttr, ent.LoggerName))
	all := make([]zapcore.Field, 0, len(c.fields)+len(fields))
	all = append(all, c.fields...)
	all = append(all, fields...)
	for _, f := range all {
		// A field can add more than one key, as zap.Error does.
		enc := zapcore.NewMapObjectEncoder()
		f.AddTo(enc)
		for k, v := range enc.Fields {
			r.AddAttrs(slog.Any(k, v))
		}
	}
	return c.h.Handle(context.Background(), r)
}

func (*fscCore) Sync() error { return nil }

func (c *fscCore) With(fields []zapcore.Field) zapcore.Core {
	n := *c
	n.fields = append(append([]zapcore.Field{}, c.fields...), fields...)
	return &n
}

// slogLevel converts a zap level to a slog level. zap's levels are 1 apart and slog's are 4.
func slogLevel(lvl zapcore.Level) slog.Level {
	return slog.Level(lvl) * (slog.LevelInfo - slog.LevelDebug)
}
