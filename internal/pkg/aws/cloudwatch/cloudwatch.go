// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package cloudwatch provides a client to make API requests to Amazon CloudWatch Service.
package cloudwatch

import (
	"context"
	"fmt"
	"strings"
	"time"

	rg "github.com/aproint/copilot-cli/internal/pkg/aws/resourcegroups"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/dustin/go-humanize"
)

const (
	cloudwatchResourceType = "cloudwatch:alarm"
	compositeAlarmType     = "Composite"
	metricAlarmType        = "Metric"
)

// humanizeDuration is overridden in tests so that its output is constant as time passes.
var humanizeDuration = humanize.RelTime

type api interface {
	DescribeAlarms(context.Context, *cloudwatch.DescribeAlarmsInput, ...func(*cloudwatch.Options)) (*cloudwatch.DescribeAlarmsOutput, error)
}

type resourceGetter interface {
	GetResourcesByTags(resourceType string, tags map[string]string) ([]*rg.Resource, error)
}

// CloudWatch wraps an Amazon CloudWatch client.
type CloudWatch struct {
	client   api
	rgClient resourceGetter
}

// AlarmStatus contains CloudWatch alarm status.
type AlarmStatus struct {
	Arn          string    `json:"arn"`
	Name         string    `json:"name"`
	Condition    string    `json:"condition"`
	Status       string    `json:"status"`
	Type         string    `json:"type"`
	UpdatedTimes time.Time `json:"updatedTimes"`
}

// AlarmDescription contains CloudWatch alarm config.
// Also available: MetricName, ComparisonOperator, DatapointsToAlarm, EvaluationPeriods, Threshold, Unit.
type AlarmDescription struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Environment string `json:"environment"`
}

// New returns a CloudWatch struct configured against the input SDK v2 CloudWatch and Resource Groups configs.
func New(cwConfig awsv2.Config, rgConfig awsv2.Config) *CloudWatch {
	return &CloudWatch{
		client:   cloudwatch.NewFromConfig(cwConfig),
		rgClient: rg.New(rgConfig),
	}
}

// AlarmsWithTags returns the statuses of all the CloudWatch alarms that have the resource tags.
func (cw *CloudWatch) AlarmsWithTags(tags map[string]string) ([]AlarmStatus, error) {
	var alarmNames []string
	resources, err := cw.rgClient.GetResourcesByTags(cloudwatchResourceType, tags)
	if err != nil {
		return nil, err
	}
	for _, resource := range resources {
		name, err := getAlarmName(resource.ARN)
		if err != nil {
			return nil, err
		}
		alarmNames = append(alarmNames, name)
	}
	if len(alarmNames) == 0 {
		return nil, nil
	}
	return cw.AlarmStatuses(WithNames(alarmNames))
}

// DescribeAlarmOpts sets the optional parameter for DescribeAlarms
type DescribeAlarmOpts func(input *cloudwatch.DescribeAlarmsInput)

// WithNames sets DescribeAlarms to filter on alarm names.
func WithNames(names []string) DescribeAlarmOpts {
	return func(in *cloudwatch.DescribeAlarmsInput) {
		in.AlarmNames = names
	}
}

// WithPrefix sets DescribeAlarms to filter on a name prefix.
func WithPrefix(prefix string) DescribeAlarmOpts {
	return func(in *cloudwatch.DescribeAlarmsInput) {
		in.AlarmNamePrefix = awsv2.String(prefix)
	}
}

// AlarmStatuses returns the statuses of alarms optionally filtered (by name, prefix, etc.).
// If the optional parameter is passed in but is nil, the statuses of ALL alarms in the
// account will be returned!
func (cw *CloudWatch) AlarmStatuses(opts ...DescribeAlarmOpts) ([]AlarmStatus, error) {
	var alarmStatuses []AlarmStatus
	in := &cloudwatch.DescribeAlarmsInput{}
	if len(opts) > 0 {
		for _, opt := range opts {
			opt(in)
		}
	}
	for {
		alarmResp, err := cw.client.DescribeAlarms(context.Background(), in)
		if err != nil {
			return nil, fmt.Errorf("describe CloudWatch alarms: %w", err)
		}
		if alarmResp == nil {
			break
		}
		alarmStatuses = append(alarmStatuses, cw.compositeAlarmsStatus(alarmResp.CompositeAlarms)...)
		alarmStatuses = append(alarmStatuses, cw.metricAlarmsStatus(alarmResp.MetricAlarms)...)
		if alarmResp.NextToken == nil {
			break
		}
		in.NextToken = alarmResp.NextToken
	}
	return alarmStatuses, nil
}

// AlarmDescriptions returns the config of alarms filtered by name.
func (cw *CloudWatch) AlarmDescriptions(alarmNames []string) ([]*AlarmDescription, error) {
	if len(alarmNames) == 0 {
		return nil, nil
	}
	var alarmDescriptions []*AlarmDescription
	in := &cloudwatch.DescribeAlarmsInput{
		AlarmNames: alarmNames,
	}
	for {
		alarmResp, err := cw.client.DescribeAlarms(context.Background(), in)
		if err != nil {
			return nil, fmt.Errorf("describe CloudWatch alarms: %w", err)
		}
		if alarmResp == nil {
			break
		}
		alarmDescriptions = append(alarmDescriptions, cw.compositeAlarmsDescriptions(alarmResp.CompositeAlarms)...)
		alarmDescriptions = append(alarmDescriptions, cw.metricAlarmsDescriptions(alarmResp.MetricAlarms)...)
		if alarmResp.NextToken == nil {
			break
		}
		in.NextToken = alarmResp.NextToken
	}
	return alarmDescriptions, nil
}

func (cw *CloudWatch) compositeAlarmsDescriptions(alarms []types.CompositeAlarm) []*AlarmDescription {
	var alarmDescriptionList []*AlarmDescription
	for _, alarm := range alarms {
		alarmDescriptionList = append(alarmDescriptionList, &AlarmDescription{
			Name:        awsv2.ToString(alarm.AlarmName),
			Description: awsv2.ToString(alarm.AlarmDescription),
		})
	}
	return alarmDescriptionList
}

func (cw *CloudWatch) metricAlarmsDescriptions(alarms []types.MetricAlarm) []*AlarmDescription {
	var alarmDescriptionsList []*AlarmDescription
	for _, alarm := range alarms {
		alarmDescriptionsList = append(alarmDescriptionsList, &AlarmDescription{
			Name:        awsv2.ToString(alarm.AlarmName),
			Description: awsv2.ToString(alarm.AlarmDescription),
		})
	}
	return alarmDescriptionsList
}

func (cw *CloudWatch) compositeAlarmsStatus(alarms []types.CompositeAlarm) []AlarmStatus {
	var alarmStatusList []AlarmStatus
	for _, alarm := range alarms {
		alarmStatusList = append(alarmStatusList, AlarmStatus{
			Arn:          awsv2.ToString(alarm.AlarmArn),
			Name:         awsv2.ToString(alarm.AlarmName),
			Condition:    awsv2.ToString(alarm.AlarmRule),
			Status:       string(alarm.StateValue),
			Type:         compositeAlarmType,
			UpdatedTimes: *alarm.StateUpdatedTimestamp,
		})
	}
	return alarmStatusList
}

func (cw *CloudWatch) metricAlarmsStatus(alarms []types.MetricAlarm) []AlarmStatus {
	var alarmStatusList []AlarmStatus
	for _, alarm := range alarms {
		metricAlarm := metricAlarm(alarm)
		alarmStatusList = append(alarmStatusList, AlarmStatus{
			Arn:          awsv2.ToString(metricAlarm.AlarmArn),
			Name:         awsv2.ToString(metricAlarm.AlarmName),
			Condition:    metricAlarm.condition(),
			Status:       string(metricAlarm.StateValue),
			Type:         metricAlarmType,
			UpdatedTimes: *metricAlarm.StateUpdatedTimestamp,
		})
	}
	return alarmStatusList
}

// getAlarmName gets the alarm name given a specific alarm ARN.
// For example: arn:aws:cloudwatch:us-west-2:1234567890:alarm:SDc-ReadCapacityUnitsLimit-BasicAlarm
// returns SDc-ReadCapacityUnitsLimit-BasicAlarm
func getAlarmName(alarmARN string) (string, error) {
	resp, err := arn.Parse(alarmARN)
	if err != nil {
		return "", fmt.Errorf("parse alarm ARN %s: %w", alarmARN, err)
	}
	alarmNameList := strings.Split(resp.Resource, ":")
	if len(alarmNameList) != 2 {
		return "", fmt.Errorf("unknown ARN resource format %s", resp.Resource)
	}
	return alarmNameList[1], nil
}
