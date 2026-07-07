// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package partitions

import (
	"errors"
	"testing"

	"github.com/aproint/copilot-cli/internal/pkg/aws/apprunner"
	"github.com/aproint/copilot-cli/internal/pkg/aws/ecs"
	"github.com/aproint/copilot-cli/internal/pkg/aws/s3"

	"github.com/stretchr/testify/require"
)

func TestRegion_Partition(t *testing.T) {
	testCases := map[string]struct {
		region    string
		wantedErr error
	}{
		"error finding the partition": {
			region:    "weird region",
			wantedErr: errors.New("find the partition for region weird region"),
		},
		"success": {
			region: "us-west-2",
		},
		"success with newer commercial partition region": {
			region: "mx-central-1",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			_, err := Region(tc.region).Partition()
			if tc.wantedErr != nil {
				require.EqualError(t, err, tc.wantedErr.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestRegion_IsAvailableInRegion(t *testing.T) {
	testCases := map[string]struct {
		sID       string
		region    string
		want      bool
		wantedErr error
	}{
		"ecs service exist in the given region": {
			region: "us-west-2",
			sID:    ecs.EndpointsID,
			want:   true,
		},
		"ecs service exists in newer SDK endpoint region": {
			region: "ap-southeast-5",
			sID:    ecs.EndpointsID,
			want:   true,
		},
		"ecs service exists in newer commercial partition region": {
			region: "mx-central-1",
			sID:    ecs.EndpointsID,
			want:   true,
		},
		"ecs service does not exist in the given region": {
			region: "us-west-3",
			sID:    ecs.EndpointsID,
			want:   false,
		},
		"s3 service exists in newer SDK endpoint region": {
			region: "ap-southeast-5",
			sID:    s3.EndpointsID,
			want:   true,
		},
		"s3 service exists in newer commercial partition region": {
			region: "mx-central-1",
			sID:    s3.EndpointsID,
			want:   true,
		},
		"apprunner service exist in the given region": {
			region: "us-west-2",
			sID:    apprunner.EndpointsID,
			want:   true,
		},
		"apprunner service does not exist in the given region": {
			region: "us-west-1",
			sID:    apprunner.EndpointsID,
			want:   false,
		},
		"unknown service does not exist in the given region": {
			region: "us-west-2",
			sID:    "some-service",
			want:   false,
		},
		"error finding the partition": {
			region:    "weird region",
			sID:       ecs.EndpointsID,
			wantedErr: errors.New("find the partition for region weird region"),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, err := IsAvailableInRegion(tc.sID, tc.region)
			if tc.wantedErr != nil {
				require.EqualError(t, err, tc.wantedErr.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, got)
			}
		})
	}
}
