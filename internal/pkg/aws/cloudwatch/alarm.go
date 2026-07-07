// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package cloudwatch

import (
	"fmt"
	"strings"
	"time"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

const (
	anomalyDetectionBandExpression = "ANOMALY_DETECTION_BAND"
	// {metricTitle} {breachingRelationship} {threshold} for {datapointsCount} datapoints within {duration}
	fmtStaticMetricCondition = "%s %s %.2f for %d datapoints within %s"
)

type alarmThresholdTypes int

const (
	predictive alarmThresholdTypes = iota
	dynamic
	static
)

type comparisonOperator string

func (c comparisonOperator) humanString() string {
	switch c {
	case comparisonOperator(types.ComparisonOperatorGreaterThanOrEqualToThreshold):
		return "≥"
	case comparisonOperator(types.ComparisonOperatorGreaterThanThreshold):
		return ">"
	case comparisonOperator(types.ComparisonOperatorLessThanThreshold):
		return "<"
	case comparisonOperator(types.ComparisonOperatorLessThanOrEqualToThreshold):
		return "≤"
	case comparisonOperator(types.ComparisonOperatorLessThanLowerOrGreaterThanUpperThreshold):
		return "outside"
	case comparisonOperator(types.ComparisonOperatorLessThanLowerThreshold):
		return "<"
	case comparisonOperator(types.ComparisonOperatorGreaterThanUpperThreshold):
		return ">"
	default:
		return ""
	}
}

type metricAlarm types.MetricAlarm

func (a metricAlarm) condition() string {
	thresholdType := a.alarmThresholdType()
	metricName := awsv2.ToString(a.MetricName)
	period := int64(awsv2.ToInt32(a.Period))
	evaluationPeriod := int64(awsv2.ToInt32(a.EvaluationPeriods))
	datapointsToAlarm := int64(awsv2.ToInt32(a.DatapointsToAlarm))
	if datapointsToAlarm == 0 {
		// https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/aws-properties-cw-alarm.html#cfn-cloudwatch-alarm-datapointstoalarm
		datapointsToAlarm = evaluationPeriod
	}
	operator := comparisonOperator(a.ComparisonOperator)
	switch thresholdType {
	case static:
		return fmt.Sprintf(fmtStaticMetricCondition, metricName, operator.humanString(),
			awsv2.ToFloat64(a.Threshold), datapointsToAlarm, humanizePeriod(evaluationPeriod, period))
	default:
		return "-"
	}
}

func (a metricAlarm) alarmThresholdType() alarmThresholdTypes {
	if a.ThresholdMetricId == nil || len(a.Metrics) < 2 {
		return static
	}
	thresholdMetric := a.thresholdMetric()
	if thresholdMetric != nil {
		if strings.HasPrefix(awsv2.ToString(thresholdMetric.Expression), anomalyDetectionBandExpression) {
			return predictive
		}
	}
	return dynamic
}

func (a metricAlarm) thresholdMetric() *types.MetricDataQuery {
	if a.ThresholdMetricId == nil {
		return nil
	}
	for idx := range a.Metrics {
		if awsv2.ToString(a.Metrics[idx].Id) == awsv2.ToString(a.ThresholdMetricId) {
			return &a.Metrics[idx]
		}
	}
	return nil
}

func humanizePeriod(evaluationPeriod, period int64) string {
	durationPeriod := time.Duration(evaluationPeriod*period) * time.Second
	return strings.TrimSpace(humanizeDuration(time.Now(), time.Now().Add(durationPeriod), "", ""))
}
