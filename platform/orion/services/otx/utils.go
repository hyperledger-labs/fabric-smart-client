/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package otx

func UsersMap(users ...string) map[string]bool {
	m := make(map[string]bool)
	for _, u := range users {
		m[u] = true
	}
	return m
}
