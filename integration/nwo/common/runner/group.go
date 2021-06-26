/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package runner

import (
	"os"

	"github.com/tedsuo/ifrit/grouper"
)

type GroupRunner interface {
	Run(signals <-chan os.Signal, ready chan<- struct{}) error
	Members() grouper.Members
}
