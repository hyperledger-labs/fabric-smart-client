/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

// Asset struct and properties must be exported (start with capitals) to work with contract api metadata
type Asset struct {
	AppraisedValue int    `json:"AppraisedValue"`
	Color          string `json:"Color"`
	ID             string `json:"ID"`
	Owner          string `json:"Owner"`
	Size           int    `json:"Size"`
}
