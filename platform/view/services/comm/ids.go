/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import "encoding/base64"

func computeInternalSessionID(topic, endpoint string, pkid []byte) string {
	return topic + "." + base64.StdEncoding.EncodeToString(pkid)
	//return topic + "." + endpoint
}
