/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package logging

import (
	"io"
	"sync"

	"github.com/hyperledger/fabric-lib-go/common/flogging"
)

type Config struct {
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

var (
	config      = Config{}
	configMutex = sync.RWMutex{}
)

func Init(c Config) {
	flogging.Init(flogging.Config{
		Format:  c.Format,
		LogSpec: c.LogSpec,
		Writer:  c.Writer,
	})

	// set local configurations
	configMutex.Lock()
	defer configMutex.Unlock()
	config.OtelSanitize = c.OtelSanitize
}

func OtelSanitize() bool {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return config.OtelSanitize
}

var (
	replacersMutex sync.RWMutex
	replacers      = map[string]string{
		"github.com_hyperledger-labs_fabric-smart-client_platform": "fsc",
	}
)

// RegisterReplacer registers a new replacer for the logger name
func RegisterReplacer(s string, replaceWith string) {
	replacersMutex.Lock()
	defer replacersMutex.Unlock()

	_, ok := replacers[s]
	if ok {
		panic("replacer already exists")
	}

	replacers[s] = replaceWith
}

// Replacers returns the current replacers
func Replacers() map[string]string {
	replacersMutex.RLock()
	defer replacersMutex.RUnlock()
	return replacers
}
