/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"fmt"
	"path"
	"time"
)

type TestOpts struct {
	name    string
	tempDir string
}

func (o TestOpts) DataSource() string {
	return fmt.Sprintf("file:%s.sqlite?_pragma=busy_timeout(1000)", path.Join(o.tempDir, o.name))
}
func (o TestOpts) SkipPragmas() bool          { return false }
func (o TestOpts) SkipCreateTable() bool      { return false }
func (o TestOpts) MaxOpenConns() int          { return 0 }
func (o TestOpts) MaxIdleConns() int          { return 2 }
func (o TestOpts) MaxIdleTime() time.Duration { return time.Minute }
