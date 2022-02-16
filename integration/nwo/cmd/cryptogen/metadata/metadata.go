/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metadata

import (
	"fmt"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/metadata"
	"runtime"
)

const ProgramName = "cryptogen"

var (
	CommitSHA = metadata.CommitSHA
	Version   = metadata.Version
)

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
