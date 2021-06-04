/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package runner

import "github.com/tedsuo/ifrit/grouper"

type Members interface {
	Members() grouper.Members
	Stop(name string) grouper.ErrorTrace
}
