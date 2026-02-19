/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"context"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/stretchr/testify/require"
)

func TestCompileRunViewOptions(t *testing.T) {
	ctx1 := t.Context()
	ctx2, cancel := context.WithCancel(ctx1)
	t.Cleanup(cancel)

	callFn := func(Context) (interface{}, error) { return "ok", nil }
	resetInitiator := func(o *RunViewOptions) error {
		o.AsInitiator = false
		return nil
	}

	tests := []struct {
		name        string
		opts        []RunViewOption
		check       func(t *testing.T, opts *RunViewOptions)
		expectedErr string
	}{
		{
			name: "No_Options_Returns_Zero_Value",
			opts: nil,
			check: func(t *testing.T, opts *RunViewOptions) {
				require.NotNil(t, opts)
				require.Nil(t, opts.Session)
				require.False(t, opts.AsInitiator)
				require.Nil(t, opts.Call)
				require.False(t, opts.SameContext)
				require.Nil(t, opts.Ctx)
			},
		},
		{
			name: "AsInitiator_Sets_Flag",
			opts: []RunViewOption{AsInitiator()},
			check: func(t *testing.T, opts *RunViewOptions) {
				require.True(t, opts.AsInitiator)
			},
		},
		{
			name: "AsResponder_Sets_Session",
			opts: []RunViewOption{AsResponder(nil)},
			check: func(t *testing.T, opts *RunViewOptions) {
				require.Nil(t, opts.Session)
			},
		},
		{
			name: "WithViewCall_Sets_Call_Function",
			opts: []RunViewOption{WithViewCall(callFn)},
			check: func(t *testing.T, opts *RunViewOptions) {
				require.NotNil(t, opts.Call)
				result, err := opts.Call(nil)
				require.NoError(t, err)
				require.Equal(t, "ok", result)
			},
		},
		{
			name: "WithSameContext_Sets_Flag_Without_Ctx",
			opts: []RunViewOption{WithSameContext()},
			check: func(t *testing.T, opts *RunViewOptions) {
				require.True(t, opts.SameContext)
				require.Nil(t, opts.Ctx)
			},
		},
		{
			name: "WithContext_Sets_Both_SameContext_And_Ctx",
			opts: []RunViewOption{WithContext(ctx1)},
			check: func(t *testing.T, opts *RunViewOptions) {
				require.True(t, opts.SameContext)
				require.Equal(t, ctx1, opts.Ctx)
			},
		},
		{
			name: "Multiple_Options_All_Applied_In_Order",
			opts: []RunViewOption{AsInitiator(), WithViewCall(callFn), WithContext(ctx1)},
			check: func(t *testing.T, opts *RunViewOptions) {
				require.True(t, opts.AsInitiator)
				require.NotNil(t, opts.Call)
				require.True(t, opts.SameContext)
				require.Equal(t, ctx1, opts.Ctx)
			},
		},
		{
			name: "Later_Option_Overrides_Earlier",
			opts: []RunViewOption{AsInitiator(), resetInitiator},
			check: func(t *testing.T, opts *RunViewOptions) {
				require.False(t, opts.AsInitiator)
			},
		},
		{
			name: "Later_WithContext_Overrides_Earlier",
			opts: []RunViewOption{WithContext(ctx1), WithContext(ctx2)},
			check: func(t *testing.T, opts *RunViewOptions) {
				require.Equal(t, ctx2, opts.Ctx)
			},
		},
		{
			name:        "Stops_On_First_Error",
			opts:        []RunViewOption{WithSameContext(), func(o *RunViewOptions) error { return errors.New("option failed") }, AsInitiator()},
			expectedErr: "option failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts, err := CompileRunViewOptions(tt.opts...)

			if tt.expectedErr != "" {
				require.EqualError(t, err, tt.expectedErr)
				require.Nil(t, opts)
				return
			}

			require.NoError(t, err)
			if tt.check != nil {
				tt.check(t, opts)
			}
		})
	}
}
