/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package runner adapts batch-processing functions to individual Run and Execute
// calls. Callers can process each input immediately or collect inputs into batches
// triggered by a capacity limit or timeout.
package runner
