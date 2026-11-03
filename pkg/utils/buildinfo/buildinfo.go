/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package buildinfo reports what the Go toolchain embedded in the running binary about the
// FSC module, the application module and the VCS revision.
package buildinfo

import (
	"cmp"
	"runtime/debug"
	"sync"
)

// FSCModule is the module path of the Fabric Smart Client.
const FSCModule = "github.com/hyperledger-labs/fabric-smart-client"

// devel is what Go reports for a module built from a local directory.
const devel = "(devel)"

// Info is what the binary's build info says about its modules.
type Info struct {
	// FSCVersion is the FSC module's version: the dependency's version, or the main module's
	// when FSC is the main module. A replaced dependency reports the replacement's version. It
	// is "(devel)" when FSC is replaced by a local directory or the binary carries no build
	// info.
	FSCVersion string
	// AppModule and AppVersion are the main module's path and version. A main module built
	// from a checkout without a VCS tag reports a pseudo-version or "(devel)".
	AppModule, AppVersion string
	// Revision and Modified are the main module's vcs.revision and vcs.modified build
	// settings; Revision is empty when the build recorded no VCS information.
	Revision string
	Modified bool
}

// Get returns the Info of the running binary. The build info never changes while the binary
// runs, so it is read once.
var Get = sync.OnceValue(func() Info { return FromBuildInfo(debug.ReadBuildInfo()) })

// FromBuildInfo extracts Info from bi, as debug.ReadBuildInfo returns it.
func FromBuildInfo(bi *debug.BuildInfo, ok bool) Info {
	info := Info{FSCVersion: devel}
	if !ok {
		return info
	}
	info.AppModule, info.AppVersion = bi.Main.Path, bi.Main.Version
	if bi.Main.Path == FSCModule {
		info.FSCVersion = cmp.Or(bi.Main.Version, info.FSCVersion)
	}
	for _, d := range bi.Deps {
		if d.Path == FSCModule {
			if d.Replace != nil {
				d = d.Replace
			}
			info.FSCVersion = cmp.Or(d.Version, info.FSCVersion)
		}
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			info.Revision = s.Value
		case "vcs.modified":
			info.Modified = s.Value == "true"
		}
	}
	return info
}
