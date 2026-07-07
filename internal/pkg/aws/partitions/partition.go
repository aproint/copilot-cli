// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package partitions

import (
	"fmt"
	"reflect"
	"regexp"

	copilotapprunner "github.com/aproint/copilot-cli/internal/pkg/aws/apprunner"
	copilotecs "github.com/aproint/copilot-cli/internal/pkg/aws/ecs"
	copilots3 "github.com/aproint/copilot-cli/internal/pkg/aws/s3"
	sdkapprunner "github.com/aws/aws-sdk-go-v2/service/apprunner"
	sdkecs "github.com/aws/aws-sdk-go-v2/service/ecs"
	sdks3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

var (
	awsRegionRegex      = regexp.MustCompile(`^(us|eu|ap|sa|ca|me|af|il|mx)-\w+-\d+$`)
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
	if _, err := Region(region).Partition(); err != nil {
		return false, err
	}

	switch sID {
	case copilotapprunner.EndpointsID:
		return resolverHasRegion(sdkapprunner.NewDefaultEndpointResolver(), region)
	case copilotecs.EndpointsID:
		return resolverHasRegion(sdkecs.NewDefaultEndpointResolver(), region)
	case copilots3.EndpointsID:
		return resolverHasRegion(sdks3.NewDefaultEndpointResolver(), region)
	default:
		return false, nil
	}
}

func resolverHasRegion(resolver any, region string) (bool, error) {
	partitions := reflect.ValueOf(resolver)
	if !partitions.IsValid() {
		return false, fmt.Errorf("inspect endpoint resolver metadata")
	}
	if partitions.Kind() == reflect.Pointer {
		if partitions.IsNil() {
			return false, fmt.Errorf("inspect endpoint resolver metadata")
		}
		partitions = partitions.Elem()
	}
	partitions = partitions.FieldByName("partitions")
	if !partitions.IsValid() || partitions.Kind() != reflect.Slice {
		return false, fmt.Errorf("inspect endpoint resolver metadata")
	}

	for i := 0; i < partitions.Len(); i++ {
		endpoints := partitions.Index(i).FieldByName("Endpoints")
		if !endpoints.IsValid() || endpoints.Kind() != reflect.Map {
			return false, fmt.Errorf("inspect endpoint resolver metadata")
		}
		for _, key := range endpoints.MapKeys() {
			if key.Kind() != reflect.Struct {
				return false, fmt.Errorf("inspect endpoint resolver metadata")
			}
			regionField := key.FieldByName("Region")
			if !regionField.IsValid() || regionField.Kind() != reflect.String {
				return false, fmt.Errorf("inspect endpoint resolver metadata")
			}
			if regionField.String() == region {
				return true, nil
			}
		}
	}
	return false, nil
}
