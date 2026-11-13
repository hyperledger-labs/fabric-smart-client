/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync/atomic"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
	"go.uber.org/zap/exp/zapslog"
	"go.uber.org/zap/zapcore"
)

// loggerAttr is the attribute under which a handler receives the logger name.
const loggerAttr = "logger"

// backend holds the handler loggers write through. A new backend is stored on each
// SetHandler, and a logger compares its pointer to know when to derive its handler again.
type backend struct{ h slog.Handler }

var (
	defaultBackend = &backend{h: newZapHandler(flogging.Global.ZapLogger("fsc").Core())}
	current        atomic.Pointer[backend]
)

func init() { current.Store(defaultBackend) }

// SetHandler makes h the backend of every FSC logger, including loggers created before
// the call. SetHandler(nil) restores the default handler, which writes in the format and
// to the writer that [Init] sets.
//
// With a handler set, logging.format and Config.Writer are ignored; logging.spec and
// logging.otel.sanitize still apply. A line reaches h only if logging.spec enables it
// for the logger's name and h.Enabled reports true, so to let h alone decide, set
// logging.spec to debug. The handler receives the logger name as the attribute "logger".
//
// Call SetHandler before building the node, so that every line goes through h. It is
// safe to call while other goroutines log. h must not write through [Handler], directly
// or through slog.Default after slog.SetDefault(slog.New(Handler(name))): the line would
// come back to h without end.
func SetHandler(h slog.Handler) {
	if h == nil {
		current.Store(defaultBackend)
		return
	}
	current.Store(&backend{h: h})
}

// handlerName names the current handler for the line Init logs.
func handlerName() string {
	if b := current.Load(); b != defaultBackend {
		return fmt.Sprintf("%T", b.h)
	}
	return "default"
}

// Handler returns a slog.Handler that checks logging.spec for logger name name and then
// forwards to the current handler (see [SetHandler]), so an application can route its
// own slog lines through FSC's levels and handler:
//
//	slog.SetDefault(slog.New(logging.Handler("app")))
func Handler(name string) slog.Handler {
	return &specHandler{name: name, cache: &handlerCache{derive: func(h slog.Handler) slog.Handler {
		return h.WithAttrs([]slog.Attr{slog.String(loggerAttr, name)})
	}}}
}

type specHandler struct {
	name  string
	cache *handlerCache
}

func (h *specHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return specEnabled(flogging.Global.LoggerLevels, h.name, level) && h.cache.get().Enabled(ctx, level)
}

func (h *specHandler) Handle(ctx context.Context, r slog.Record) error {
	return h.cache.get().Handle(ctx, r)
}

func (h *specHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h.then(func(x slog.Handler) slog.Handler { return x.WithAttrs(attrs) })
}

func (h *specHandler) WithGroup(name string) slog.Handler {
	return h.then(func(x slog.Handler) slog.Handler { return x.WithGroup(name) })
}

// then returns a handler that derives like h and then applies f, so WithAttrs and
// WithGroup are applied again when SetHandler changes the handler.
func (h *specHandler) then(f func(slog.Handler) slog.Handler) slog.Handler {
	derive := h.cache.derive
	return &specHandler{name: h.name, cache: &handlerCache{derive: func(x slog.Handler) slog.Handler {
		return f(derive(x))
	}}}
}

// specEnabled reports whether logging.spec enables level for name. Nil levels enable all.
func specEnabled(levels *flogging.LoggerLevels, name string, level slog.Level) bool {
	return levels == nil || levels.Level(name).Enabled(zapLevel(level))
}

// handlerCache derives a handler from a backend once and derives it again when the
// backend changes, so a logger pays for WithAttrs only after SetHandler.
type handlerCache struct {
	// root is the backend to derive from; nil means the current one.
	root   *backend
	derive func(slog.Handler) slog.Handler
	last   atomic.Pointer[derivedHandler]
}

type derivedHandler struct {
	from *backend
	h    slog.Handler
}

func (c *handlerCache) get() slog.Handler {
	b := c.root
	if b == nil {
		b = current.Load()
	}
	if d := c.last.Load(); d != nil && d.from == b {
		return d.h
	}
	d := &derivedHandler{from: b, h: c.derive(b.h)}
	c.last.Store(d)
	return d.h
}

// zapStackSkip makes zapslog's stack trace at ErrorLevel start at the caller of the
// Logger method, or of the slog.Logger method for a line through [Handler]: zapslog
// skips the frames of slog.Logger, and both paths have one frame more before Handle.
const zapStackSkip = 1

// zapHandler is the default handler: zapslog over a flogging core, so lines have
// flogging's format, writer and per-name levels. It takes the "logger" attribute, which
// a logger passes first, as zap's logger name, which flogging prints and checks levels
// against, instead of writing it as a field.
type zapHandler struct {
	*zapslog.Handler
	core zapcore.Core
}

func newZapHandler(core zapcore.Core) *zapHandler {
	return &zapHandler{Handler: newZapslog(core, ""), core: core}
}

func newZapslog(core zapcore.Core, name string) *zapslog.Handler {
	return zapslog.NewHandler(core, zapslog.WithName(name), zapslog.WithCaller(true), zapslog.WithCallerSkip(zapStackSkip))
}

func (h *zapHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 || attrs[0].Key != loggerAttr {
		return h.Handler.WithAttrs(attrs)
	}
	return newZapslog(h.core, attrs[0].Value.String()).WithAttrs(slices.Clip(attrs[1:]))
}
