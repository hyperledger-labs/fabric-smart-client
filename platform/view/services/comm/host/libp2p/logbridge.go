/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package libp2p

import (
	"log/slog"

	"github.com/libp2p/go-libp2p/gologshim"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/logging"
)

// init sends libp2p's lines, which go-libp2p logs through gologshim, to FSC logging
// under the logger name libp2p.
//
// A gologshim logger keeps the handler of its first line, so one that logs before
// this init, as tcp-demultiplex does with LIBP2P_TCP_REUSEPORT false, stays on stderr.
func init() {
	gologshim.SetDefaultHandler(subsystemHandler{logging.Handler("libp2p")})
}

// subsystemHandler passes the "logger" attribute that gologshim adds, which holds
// the subsystem, such as basichost, as "subsystem", so it doesn't clash with the
// logger name FSC passes under the same key. gologshim adds it with the one
// WithAttrs call it makes when it binds the handler.
type subsystemHandler struct{ slog.Handler }

func (h subsystemHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	for i, a := range attrs {
		if a.Key == "logger" {
			attrs[i].Key = "subsystem"
		}
	}
	return h.Handler.WithAttrs(attrs)
}
