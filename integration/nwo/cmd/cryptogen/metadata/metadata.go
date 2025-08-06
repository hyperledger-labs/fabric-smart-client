/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metadata

import (
	"fmt"
	"runtime"
)

const ProgramName = "cryptogen"

// Variables defined by the Makefile and passed in with ldflags
var Version = "latest"
var CommitSHA = "development build"

func GetVersionInfo() string {
	return fmt.Sprintf(
		"%s:\n Version: %s\n Commit SHA: %s\n Go version: %s\n OS/Arch: %s",
		ProgramName,
		Version,
		CommitSHA,
		runtime.Version(),
		fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	)
}
