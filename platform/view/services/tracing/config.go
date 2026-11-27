/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tracing

import (
	"fmt"
	"maps"
	"runtime"
	"strings"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/hyperledger-labs/fabric-smart-client/internal/names"
)

const (
	namespaceKey  = "namespace"
	subsystemKey  = "subsystem"
	labelNamesKey = "label_names"
)

type LabelName = string

type MetricsOpts struct {
	Namespace  string
	Subsystem  string
	LabelNames []LabelName
}

var (
	replacersMutex sync.RWMutex
	replacers      = map[string]string{
		"github.com_hyperledger-labs_fabric-smart-client_platform": "fsc",
	}
)

// RegisterReplacer makes the namespace derived from a package path replace s with
// replaceWith wherever s occurs, for names derived after the call. Where registered
// strings overlap at the same position, the longest one is replaced. It panics if s is
// already registered.
func RegisterReplacer(s, replaceWith string) {
	replacersMutex.Lock()
	defer replacersMutex.Unlock()

	_, ok := replacers[s]
	if ok {
		panic("replacer already exists")
	}

	replacers[s] = replaceWith
}

// Replacers returns a snapshot of the registered replacer map. The returned
// map is a copy: callers can read or mutate it freely without affecting the
// internal state or racing with concurrent RegisterReplacer calls.
func Replacers() map[string]string {
	replacersMutex.RLock()
	defer replacersMutex.RUnlock()
	return maps.Clone(replacers)
}

func WithMetricsOpts(o MetricsOpts) trace.TracerOption {
	namespace, subsystem := parseFullPkgName(GetPackageName(), Replacers())
	if len(o.Namespace) == 0 {
		o.Namespace = namespace
	}
	if len(o.Subsystem) == 0 {
		o.Subsystem = subsystem
	}
	set := attribute.NewSet(
		attribute.String(namespaceKey, o.Namespace),
		attribute.String(subsystemKey, o.Subsystem),
		attribute.StringSlice(labelNamesKey, o.LabelNames),
	)
	return trace.WithInstrumentationAttributes(set.ToSlice()...)
}

func extractMetricsOpts(attrs attribute.Set) MetricsOpts {
	o := MetricsOpts{}
	if val, ok := attrs.Value(namespaceKey); ok {
		o.Namespace = val.AsString()
	}
	if val, ok := attrs.Value(subsystemKey); ok {
		o.Subsystem = val.AsString()
	}
	if val, ok := attrs.Value(labelNamesKey); ok {
		o.LabelNames = val.AsStringSlice()
	}
	return o
}

func GetPackageName() string {
	pc, _, _, ok := runtime.Caller(2)
	if !ok {
		panic("GetPackageName: unable to retrieve caller information using runtime.Caller")
	}
	fn := runtime.FuncForPC(pc)
	if fn == nil {
		panic(fmt.Sprintf("GetPackageName: unable to retrieve function for PC: %v", pc))
	}
	fullFuncName := fn.Name()
	lastSlash := strings.LastIndex(fullFuncName, "/")
	if lastSlash == -1 {
		return fullFuncName
	}
	dotAfterSlash := strings.Index(fullFuncName[lastSlash:], ".")
	return fullFuncName[:lastSlash+dotAfterSlash]
}

func parseFullPkgName(fullPkgName string, replacements map[string]string, params ...string) (namespace, subsystem string) {
	parts := append(strings.Split(fullPkgName, "/"), params...)
	subsystem = parts[len(parts)-1]
	namespaceParts := parts[:len(parts)-1]
	namespace = names.ReplaceLongestFirst(strings.Join(namespaceParts, "_"), replacements)
	return namespace, subsystem
}
