/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"cmp"
	"io"
	"os"
	"sync"
	"sync/atomic"

	"github.com/hyperledger/fabric-lib-go/common/flogging"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// Config configures the logging of the process; see [Init].
type Config struct {
	// Format is the log line format, logging.format in core.yaml. "json" writes JSON
	// lines; any other value is a pattern of verbs such as %{time} and %{message}, as
	// in Fabric. Empty means Fabric's default pattern.
	Format string
	// LogSpec sets the level of each logger, logging.spec in core.yaml, in the form
	// [<logger>[,<logger>...]=]<level>[:[<logger>[,<logger>...]=]<level>...]. Empty
	// means the FABRIC_LOGGING_SPEC environment variable, or info if that is unset.
	LogSpec string
	// Writer receives the formatted log lines. Nil keeps the current writer, which is
	// os.Stderr until a call to Init sets another one.
	Writer io.Writer

	// OtelSanitize, logging.otel.sanitize in core.yaml, removes non-printable characters
	// from the messages recorded as span events, and prefixes such a message with
	// "[nonprintable] ". Non-printable characters can break a tracing backend.
	OtelSanitize bool

	// ContextLogFields lists context keys to add as fields to every line written by a
	// context-aware method; see [RegisterContextLogField].
	ContextLogFields []ContextLogField
}

// ContextLogField maps a context key to the field name its value is logged under.
type ContextLogField struct {
	// Key is looked up with ctx.Value(Key).
	Key any
	// Name is the field name the value is logged under.
	Name string
}

var otelSanitizeEnabled atomic.Bool

// Init (re)configures logging for the whole process, including loggers created before
// the call. Format, LogSpec and OtelSanitize replace the earlier settings, and an empty
// Format or LogSpec means its default, not the earlier value: to change only the writer,
// pass the whole configuration again. A nil Writer keeps the current writer.
// ContextLogFields are added to the registered ones, a field with a name already
// registered replacing its key.
//
// Init fails if Format or LogSpec is invalid, and then changes nothing.
func Init(c Config) error {
	spec, specSource := c.LogSpec, "logging.spec"
	if spec == "" {
		spec, specSource = cmp.Or(os.Getenv("FABRIC_LOGGING_SPEC"), "info"), "FABRIC_LOGGING_SPEC"
	}
	// ActivateSpec validates before it changes anything; checking on a scratch copy
	// first means an invalid spec doesn't leave a new format behind.
	if err := (&flogging.LoggerLevels{}).ActivateSpec(spec); err != nil {
		return errors.Wrapf(err, "invalid %s", specSource)
	}
	if err := flogging.Global.SetFormat(c.Format); err != nil {
		return errors.Wrapf(err, "invalid logging.format [%s]", c.Format)
	}
	if err := flogging.Global.ActivateSpec(spec); err != nil {
		return errors.Wrapf(err, "invalid %s", specSource)
	}
	if c.Writer != nil {
		flogging.Global.SetWriter(c.Writer)
	}

	otelSanitizeEnabled.Store(c.OtelSanitize)

	for _, f := range c.ContextLogFields {
		doRegisterContextLogField(f.Name, f.Key, true)
	}
	return nil
}

func otelSanitize() bool { return otelSanitizeEnabled.Load() }

var (
	replacersMutex sync.RWMutex
	replacers      = map[string]string{
		"github.com.hyperledger-labs.fabric-smart-client": "fsc",
	}
)

// RegisterReplacer makes [GetLogger] replace s with replaceWith in the names of loggers
// created after the call. It panics if s is already registered.
func RegisterReplacer(s, replaceWith string) {
	replacersMutex.Lock()
	defer replacersMutex.Unlock()

	if _, ok := replacers[s]; ok {
		panic("replacer already exists")
	}
	replacers[s] = replaceWith
}

var (
	// ctxFieldsMutex guards registration (rare, startup-time) writes to ctxFieldNames and
	// ctxLogFields below. The hot read path, contextLogFields, never takes it: it loads the
	// immutable snapshot published in ctxFieldsSnap instead, so every context-aware log call
	// avoids both lock contention and a per-call copy of the registry.
	ctxFieldsMutex sync.Mutex
	ctxFieldNames  []string
	ctxLogFields   = map[string]any{}
	ctxFieldsSnap  atomic.Pointer[[]ContextLogField]
)

// RegisterContextLogField makes every context-aware log call look up key in its context
// and, if present, log the value under name. It panics if name is already registered;
// [Init] instead replaces the key of a registered name.
func RegisterContextLogField(name string, key any) {
	doRegisterContextLogField(name, key, false)
}

func doRegisterContextLogField(name string, key any, overwrite bool) {
	ctxFieldsMutex.Lock()
	defer ctxFieldsMutex.Unlock()

	if _, ok := ctxLogFields[name]; ok {
		if !overwrite {
			panic("context log field already exists")
		}
		ctxLogFields[name] = key
	} else {
		ctxLogFields[name] = key
		ctxFieldNames = append(ctxFieldNames, name)
	}

	fields := make([]ContextLogField, len(ctxFieldNames))
	for i, n := range ctxFieldNames {
		fields[i] = ContextLogField{Key: ctxLogFields[n], Name: n}
	}
	ctxFieldsSnap.Store(&fields)
}

// contextLogFields returns the registered context log fields, in registration order. The
// slice is an immutable snapshot and must not be changed.
func contextLogFields() []ContextLogField {
	if p := ctxFieldsSnap.Load(); p != nil {
		return *p
	}
	return nil
}

// resetContextLogFields clears the context log field registry, so each test can start
// from an empty one.
//
//lint:ignore U1000 used for testing
func resetContextLogFields() {
	ctxFieldsMutex.Lock()
	defer ctxFieldsMutex.Unlock()

	ctxFieldNames = nil
	ctxLogFields = map[string]any{}
	ctxFieldsSnap.Store(&[]ContextLogField{})
}
