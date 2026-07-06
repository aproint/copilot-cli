// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package partitions

import (
	"fmt"
	"regexp"
)

var (
	awsRegionRegex      = regexp.MustCompile(`^(us|eu|ap|sa|ca|me|af|il)-\w+-\d+$`)
	awsChinaRegionRegex = regexp.MustCompile(`^cn-\w+-\d+$`)
	awsGovRegionRegex   = regexp.MustCompile(`^us-gov-\w+-\d+$`)
)

// Region is an AWS region ID.
type Region string

// Partition describes the AWS partition that a region belongs to.
type Partition struct {
	id string
}

// ID returns the partition identifier.
func (p Partition) ID() string {
	return p.id
}

// Partition returns the first partition which includes the region passed in, from a list of the partitions the SDK is bundled with.
func (r Region) Partition() (Partition, error) {
	region := string(r)
	switch {
	case awsRegionRegex.MatchString(region):
		return Partition{id: "aws"}, nil
	case awsChinaRegionRegex.MatchString(region):
		return Partition{id: "aws-cn"}, nil
	case awsGovRegionRegex.MatchString(region):
		return Partition{id: "aws-us-gov"}, nil
	default:
		return Partition{}, fmt.Errorf("find the partition for region %s", region)
	}
}

// IsAvailableInRegion returns true if the service ID is available in the given region.
func IsAvailableInRegion(sID string, region string) (bool, error) {
	partition, err := Region(region).Partition()
	if err != nil {
		return false, err
	}
	regions, ok := serviceRegions[partition.ID()][sID]
	if !ok {
		return false, nil
	}
	_, existInRegion := regions[region]
	return existInRegion, nil
}

var serviceRegions = map[string]map[string]map[string]struct{}{
	"aws": {
		"apprunner": regions(
			"ap-northeast-1",
			"ap-south-1",
			"ap-southeast-1",
			"ap-southeast-2",
			"eu-central-1",
			"eu-west-1",
			"eu-west-2",
			"eu-west-3",
			"us-east-1",
			"us-east-2",
			"us-west-2",
		),
		"ecs": regions(
			"af-south-1",
			"ap-east-1",
			"ap-northeast-1",
			"ap-northeast-2",
			"ap-northeast-3",
			"ap-south-1",
			"ap-south-2",
			"ap-southeast-1",
			"ap-southeast-2",
			"ap-southeast-3",
			"ap-southeast-4",
			"ca-central-1",
			"ca-west-1",
			"eu-central-1",
			"eu-central-2",
			"eu-north-1",
			"eu-south-1",
			"eu-south-2",
			"eu-west-1",
			"eu-west-2",
			"eu-west-3",
			"il-central-1",
			"me-central-1",
			"me-south-1",
			"sa-east-1",
			"us-east-1",
			"us-east-2",
			"us-west-1",
			"us-west-2",
		),
		"s3": regions(
			"af-south-1",
			"ap-east-1",
			"ap-northeast-1",
			"ap-northeast-2",
			"ap-northeast-3",
			"ap-south-1",
			"ap-south-2",
			"ap-southeast-1",
			"ap-southeast-2",
			"ap-southeast-3",
			"ap-southeast-4",
			"ca-central-1",
			"ca-west-1",
			"eu-central-1",
			"eu-central-2",
			"eu-north-1",
			"eu-south-1",
			"eu-south-2",
			"eu-west-1",
			"eu-west-2",
			"eu-west-3",
			"il-central-1",
			"me-central-1",
			"me-south-1",
			"sa-east-1",
			"us-east-1",
			"us-east-2",
			"us-west-1",
			"us-west-2",
		),
	},
	"aws-cn": {
		"ecs": regions(
			"cn-north-1",
			"cn-northwest-1",
		),
		"s3": regions(
			"cn-north-1",
			"cn-northwest-1",
		),
	},
	"aws-us-gov": {
		"ecs": regions(
			"us-gov-east-1",
			"us-gov-west-1",
		),
		"s3": regions(
			"us-gov-east-1",
			"us-gov-west-1",
		),
	},
}

func regions(ids ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		out[id] = struct{}{}
	}
	return out
}
