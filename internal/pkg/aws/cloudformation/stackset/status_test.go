// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package stackset

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/require"
)

func TestOpStatus_IsCompleted(t *testing.T) {
	testCases := map[string]struct {
		status string
		wanted bool
	}{
		"false when queued": {
			status: string(types.StackSetOperationStatusQueued),
		},
		"false when running": {
			status: string(types.StackSetOperationStatusRunning),
		},
		"false when stopping": {
			status: string(types.StackSetOperationStatusStopping),
		},
		"true when succeeded": {
			status: string(types.StackSetOperationStatusSucceeded),
			wanted: true,
		},
		"true when stopped": {
			status: string(types.StackSetOperationStatusStopped),
			wanted: true,
		},
		"true when failed": {
			status: string(types.StackSetOperationStatusFailed),
			wanted: true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.wanted, OpStatus(tc.status).IsCompleted())
		})
	}
}

func TestOpStatus_IsSuccess(t *testing.T) {
	testCases := map[string]struct {
		status string
		wanted bool
	}{
		"false when queued": {
			status: string(types.StackSetOperationStatusQueued),
		},
		"false when running": {
			status: string(types.StackSetOperationStatusRunning),
		},
		"false when stopping": {
			status: string(types.StackSetOperationStatusStopping),
		},
		"true when succeeded": {
			status: string(types.StackSetOperationStatusSucceeded),
			wanted: true,
		},
		"false when stopped": {
			status: string(types.StackSetOperationStatusStopped),
		},
		"false when failed": {
			status: string(types.StackSetOperationStatusFailed),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.wanted, OpStatus(tc.status).IsSuccess())
		})
	}
}

func TestOpStatus_InProgress(t *testing.T) {
	testCases := map[string]struct {
		status string
		wanted bool
	}{
		"true when queued": {
			status: string(types.StackSetOperationStatusQueued),
			wanted: true,
		},
		"true when running": {
			status: string(types.StackSetOperationStatusRunning),
			wanted: true,
		},
		"true when stopping": {
			status: string(types.StackSetOperationStatusStopping),
			wanted: true,
		},
		"false when succeeded": {
			status: string(types.StackSetOperationStatusSucceeded),
		},
		"false when stopped": {
			status: string(types.StackSetOperationStatusStopped),
		},
		"false when failed": {
			status: string(types.StackSetOperationStatusFailed),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.wanted, OpStatus(tc.status).InProgress())
		})
	}
}

func TestOpStatus_IsFailure(t *testing.T) {
	testCases := map[string]struct {
		status string
		wanted bool
	}{
		"false when queued": {
			status: string(types.StackSetOperationStatusQueued),
		},
		"false when running": {
			status: string(types.StackSetOperationStatusRunning),
		},
		"false when stopping": {
			status: string(types.StackSetOperationStatusStopping),
		},
		"false when succeeded": {
			status: string(types.StackSetOperationStatusSucceeded),
		},
		"true when stopped": {
			status: string(types.StackSetOperationStatusStopped),
			wanted: true,
		},
		"true when failed": {
			status: string(types.StackSetOperationStatusFailed),
			wanted: true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.wanted, OpStatus(tc.status).IsFailure())
		})
	}
}

func TestOpStatus_String(t *testing.T) {
	var s OpStatus = "hello"
	require.Equal(t, "hello", s.String())
}

func TestInstanceStatus_IsCompleted(t *testing.T) {
	testCases := map[string]struct {
		status string
		wanted bool
	}{
		"false when pending": {
			status: string(types.StackInstanceDetailedStatusPending),
		},
		"false when running": {
			status: string(types.StackInstanceDetailedStatusRunning),
		},
		"true when succeeded": {
			status: string(types.StackInstanceDetailedStatusSucceeded),
			wanted: true,
		},
		"true when failed": {
			status: string(types.StackInstanceDetailedStatusFailed),
			wanted: true,
		},
		"true when cancelled": {
			status: string(types.StackInstanceDetailedStatusCancelled),
			wanted: true,
		},
		"true when inoperable": {
			status: string(types.StackInstanceDetailedStatusInoperable),
			wanted: true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.wanted, InstanceStatus(tc.status).IsCompleted())
		})
	}
}

func TestInstanceStatus_InProgress(t *testing.T) {
	testCases := map[string]struct {
		status string
		wanted bool
	}{
		"true when pending": {
			status: string(types.StackInstanceDetailedStatusPending),
			wanted: true,
		},
		"true when running": {
			status: string(types.StackInstanceDetailedStatusRunning),
			wanted: true,
		},
		"false when succeeded": {
			status: string(types.StackInstanceDetailedStatusSucceeded),
		},
		"false when failed": {
			status: string(types.StackInstanceDetailedStatusFailed),
		},
		"false when cancelled": {
			status: string(types.StackInstanceDetailedStatusCancelled),
		},
		"false when inoperable": {
			status: string(types.StackInstanceDetailedStatusInoperable),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.wanted, InstanceStatus(tc.status).InProgress())
		})
	}
}

func TestInstanceStatus_IsSuccess(t *testing.T) {
	testCases := map[string]struct {
		status string
		wanted bool
	}{
		"false when pending": {
			status: string(types.StackInstanceDetailedStatusPending),
		},
		"false when running": {
			status: string(types.StackInstanceDetailedStatusRunning),
		},
		"true when succeeded": {
			status: string(types.StackInstanceDetailedStatusSucceeded),
			wanted: true,
		},
		"false when failed": {
			status: string(types.StackInstanceDetailedStatusFailed),
		},
		"false when cancelled": {
			status: string(types.StackInstanceDetailedStatusCancelled),
		},
		"false when inoperable": {
			status: string(types.StackInstanceDetailedStatusInoperable),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.wanted, InstanceStatus(tc.status).IsSuccess())
		})
	}
}

func TestInstanceStatus_IsFailure(t *testing.T) {
	testCases := map[string]struct {
		status string
		wanted bool
	}{
		"false when pending": {
			status: string(types.StackInstanceDetailedStatusPending),
		},
		"false when running": {
			status: string(types.StackInstanceDetailedStatusRunning),
		},
		"false when succeeded": {
			status: string(types.StackInstanceDetailedStatusSucceeded),
		},
		"true when failed": {
			status: string(types.StackInstanceDetailedStatusFailed),
			wanted: true,
		},
		"true when cancelled": {
			status: string(types.StackInstanceDetailedStatusCancelled),
			wanted: true,
		},
		"true when inoperable": {
			status: string(types.StackInstanceDetailedStatusInoperable),
			wanted: true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.wanted, InstanceStatus(tc.status).IsFailure())
		})
	}
}

func TestInstanceStatus_String(t *testing.T) {
	var s InstanceStatus = "hello"
	require.Equal(t, "hello", s.String())
}
