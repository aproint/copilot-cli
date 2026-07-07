// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package stack

import (
	"sort"

	"github.com/aproint/copilot-cli/internal/pkg/aws/tags"
	"github.com/aws/aws-sdk-go-v2/aws"
	cloudformation "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
)

func mergeAndFlattenTags(additionalTags map[string]string, cliTags map[string]string) []*cloudformation.Tag {
	var flatTags []*cloudformation.Tag
	for k, v := range tags.Merge(additionalTags, cliTags) {
		flatTags = append(flatTags, &cloudformation.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}
	sort.SliceStable(flatTags, func(i, j int) bool { return aws.ToString(flatTags[i].Key) < aws.ToString(flatTags[j].Key) })
	return flatTags
}
