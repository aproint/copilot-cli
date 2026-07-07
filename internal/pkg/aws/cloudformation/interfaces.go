// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package cloudformation

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
)

type changeSetAPI interface {
	CreateChangeSet(context.Context, *cloudformation.CreateChangeSetInput, ...func(*cloudformation.Options)) (*cloudformation.CreateChangeSetOutput, error)
	WaitUntilChangeSetCreateComplete(context.Context, *cloudformation.DescribeChangeSetInput, time.Duration, ...func(*cloudformation.ChangeSetCreateCompleteWaiterOptions)) error
	DescribeChangeSet(context.Context, *cloudformation.DescribeChangeSetInput, ...func(*cloudformation.Options)) (*cloudformation.DescribeChangeSetOutput, error)
	ExecuteChangeSet(context.Context, *cloudformation.ExecuteChangeSetInput, ...func(*cloudformation.Options)) (*cloudformation.ExecuteChangeSetOutput, error)
	DeleteChangeSet(context.Context, *cloudformation.DeleteChangeSetInput, ...func(*cloudformation.Options)) (*cloudformation.DeleteChangeSetOutput, error)
}

type client interface {
	changeSetAPI

	GetTemplateSummary(context.Context, *cloudformation.GetTemplateSummaryInput, ...func(*cloudformation.Options)) (*cloudformation.GetTemplateSummaryOutput, error)
	DescribeStacks(context.Context, *cloudformation.DescribeStacksInput, ...func(*cloudformation.Options)) (*cloudformation.DescribeStacksOutput, error)
	DescribeStackEvents(context.Context, *cloudformation.DescribeStackEventsInput, ...func(*cloudformation.Options)) (*cloudformation.DescribeStackEventsOutput, error)
	DescribeStackResources(context.Context, *cloudformation.DescribeStackResourcesInput, ...func(*cloudformation.Options)) (*cloudformation.DescribeStackResourcesOutput, error)
	GetTemplate(context.Context, *cloudformation.GetTemplateInput, ...func(*cloudformation.Options)) (*cloudformation.GetTemplateOutput, error)
	DeleteStack(context.Context, *cloudformation.DeleteStackInput, ...func(*cloudformation.Options)) (*cloudformation.DeleteStackOutput, error)
	WaitUntilStackCreateComplete(context.Context, *cloudformation.DescribeStacksInput, time.Duration, ...func(*cloudformation.StackCreateCompleteWaiterOptions)) error
	WaitUntilStackUpdateComplete(context.Context, *cloudformation.DescribeStacksInput, time.Duration, ...func(*cloudformation.StackUpdateCompleteWaiterOptions)) error
	WaitUntilStackDeleteComplete(context.Context, *cloudformation.DescribeStacksInput, time.Duration, ...func(*cloudformation.StackDeleteCompleteWaiterOptions)) error
	CancelUpdateStack(context.Context, *cloudformation.CancelUpdateStackInput, ...func(*cloudformation.Options)) (*cloudformation.CancelUpdateStackOutput, error)
}
