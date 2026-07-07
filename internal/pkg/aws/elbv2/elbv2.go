// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package elbv2 provides a client to make API requests to Amazon Elastic Load Balancing.
package elbv2

import (
	"context"
	"errors"
	"fmt"
	"sort"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/dustin/go-humanize/english"
)

const (
	// TargetHealthStateHealthy wraps the ELBV2 health status HEALTHY.
	TargetHealthStateHealthy = string(types.TargetHealthStateEnumHealthy)
)

type api interface {
	DescribeTargetHealth(context.Context, *elbv2.DescribeTargetHealthInput, ...func(*elbv2.Options)) (*elbv2.DescribeTargetHealthOutput, error)
	DescribeRules(context.Context, *elbv2.DescribeRulesInput, ...func(*elbv2.Options)) (*elbv2.DescribeRulesOutput, error)
	DescribeLoadBalancers(context.Context, *elbv2.DescribeLoadBalancersInput, ...func(*elbv2.Options)) (*elbv2.DescribeLoadBalancersOutput, error)
	DescribeListeners(context.Context, *elbv2.DescribeListenersInput, ...func(*elbv2.Options)) (*elbv2.DescribeListenersOutput, error)
}

// ELBV2 wraps an AWS ELBV2 client.
type ELBV2 struct {
	client api
}

// New returns a ELBV2 configured against the input SDK v2 config.
func New(cfg awsv2.Config) *ELBV2 {
	return &ELBV2{
		client: elbv2.NewFromConfig(cfg),
	}
}

// ListenerRulesHostHeaders returns all the host headers for all listener rules.
func (e *ELBV2) ListenerRulesHostHeaders(ruleARNs []string) ([]string, error) {
	resp, err := e.client.DescribeRules(context.Background(), &elbv2.DescribeRulesInput{
		RuleArns: ruleARNs,
	})
	if err != nil {
		return nil, fmt.Errorf("get listener rule for %s: %w", english.WordSeries(ruleARNs, "and"), err)
	}
	if len(resp.Rules) == 0 {
		return nil, fmt.Errorf("cannot find listener rule %s", english.WordSeries(ruleARNs, "and"))
	}
	exists := struct{}{}
	hostHeaderSet := make(map[string]struct{})
	for _, rule := range resp.Rules {
		for _, condition := range rule.Conditions {
			if awsv2.ToString(condition.Field) == "host-header" {
				// Values is a legacy field that allowed specifying only a single host name.
				// The alternative is to use HostHeaderConfig for multiple values.
				// Only one of these fields should be set, but we collect from both to be safe.
				for _, value := range condition.Values {
					hostHeaderSet[value] = exists
				}
				if condition.HostHeaderConfig == nil {
					break
				}
				for _, value := range condition.HostHeaderConfig.Values {
					hostHeaderSet[value] = exists
				}
				break
			}
		}
	}
	var hostHeaders []string
	for hostHeader := range hostHeaderSet {
		hostHeaders = append(hostHeaders, hostHeader)
	}
	sort.Slice(hostHeaders, func(i, j int) bool { return hostHeaders[i] < hostHeaders[j] })
	return hostHeaders, nil
}

// Rule wraps an elbv2.Rule to add some nice functionality to it.
type Rule types.Rule

// DescribeRule returns the Rule with ruleARN.
func (e *ELBV2) DescribeRule(ctx context.Context, ruleARN string) (Rule, error) {
	resp, err := e.client.DescribeRules(ctx, &elbv2.DescribeRulesInput{
		RuleArns: []string{ruleARN},
	})
	if err != nil {
		return Rule{}, err
	} else if len(resp.Rules) == 0 {
		return Rule{}, errors.New("not found")
	}

	return Rule(resp.Rules[0]), nil
}

// HasRedirectAction returns true if the rule has a redirect action.
func (r *Rule) HasRedirectAction() bool {
	for _, action := range r.Actions {
		if action.Type == types.ActionTypeEnumRedirect {
			return true
		}
	}
	return false
}

// TargetHealth wraps up elbv2.TargetHealthDescription.
type TargetHealth types.TargetHealthDescription

// TargetsHealth returns the health status of the targets in a target group.
func (e *ELBV2) TargetsHealth(targetGroupARN string) ([]*TargetHealth, error) {
	in := &elbv2.DescribeTargetHealthInput{
		TargetGroupArn: awsv2.String(targetGroupARN),
	}
	out, err := e.client.DescribeTargetHealth(context.Background(), in)
	if err != nil {
		return nil, fmt.Errorf("describe target health for target group %s: %w", targetGroupARN, err)
	}

	ret := make([]*TargetHealth, len(out.TargetHealthDescriptions))
	for idx := range out.TargetHealthDescriptions {
		ret[idx] = (*TargetHealth)(&out.TargetHealthDescriptions[idx])
	}
	return ret, nil
}

// TargetID returns the target's ID, which is either an instance or an IP address.
func (t *TargetHealth) TargetID() string {
	return t.targetID()
}

// HealthStatus contains the health status info of a target.
type HealthStatus struct {
	TargetID          string `json:"targetID"`
	HealthDescription string `json:"description"`
	HealthState       string `json:"state"`
	HealthReason      string `json:"reason"`
}

// HealthStatus returns the health status of the target.
func (t *TargetHealth) HealthStatus() *HealthStatus {
	return &HealthStatus{
		TargetID:          t.targetID(),
		HealthDescription: awsv2.ToString(t.TargetHealth.Description),
		HealthState:       string(t.TargetHealth.State),
		HealthReason:      string(t.TargetHealth.Reason),
	}
}

func (t *TargetHealth) targetID() string {
	return awsv2.ToString(t.Target.Id)
}

// LoadBalancer contains information about a given load balancer.
type LoadBalancer struct {
	ARN            string
	Name           string
	DNSName        string
	HostedZoneID   string
	Listeners      []Listener
	Scheme         string // "internet-facing" or "internal"
	SecurityGroups []string
}

// LoadBalancer returns select information about a load balancer.
func (e *ELBV2) LoadBalancer(nameOrARN string) (*LoadBalancer, error) {
	var input *elbv2.DescribeLoadBalancersInput
	if arn.IsARN(nameOrARN) {
		input = &elbv2.DescribeLoadBalancersInput{
			LoadBalancerArns: []string{nameOrARN},
		}
	} else {
		input = &elbv2.DescribeLoadBalancersInput{
			Names: []string{nameOrARN},
		}
	}
	output, err := e.client.DescribeLoadBalancers(context.Background(), input)
	if err != nil {
		return nil, fmt.Errorf("describe load balancer %q: %w", nameOrARN, err)
	}
	if len(output.LoadBalancers) == 0 {
		return nil, fmt.Errorf("no load balancer %q found", nameOrARN)
	}
	lb := output.LoadBalancers[0]
	listeners, err := e.listeners(awsv2.ToString(lb.LoadBalancerArn))
	if err != nil {
		return nil, err
	}
	return &LoadBalancer{
		ARN:            awsv2.ToString(lb.LoadBalancerArn),
		Name:           awsv2.ToString(lb.LoadBalancerName),
		DNSName:        awsv2.ToString(lb.DNSName),
		Scheme:         string(lb.Scheme),
		HostedZoneID:   awsv2.ToString(lb.CanonicalHostedZoneId),
		Listeners:      listeners,
		SecurityGroups: append([]string(nil), lb.SecurityGroups...),
	}, nil
}

// Listener contains information about a listener.
type Listener struct {
	ARN      string
	Port     int64
	Protocol string
}

// listeners returns select information about all listeners on a given load balancer.
func (e *ELBV2) listeners(lbARN string) ([]Listener, error) {
	var listeners []Listener
	in := &elbv2.DescribeListenersInput{LoadBalancerArn: awsv2.String(lbARN)}
	for {
		output, err := e.client.DescribeListeners(context.Background(), in)
		if err != nil {
			return nil, fmt.Errorf("describe listeners on load balancer %q: %w", lbARN, err)
		}
		if output == nil {
			break
		}
		for _, listener := range output.Listeners {
			listeners = append(listeners, Listener{
				ARN:      awsv2.ToString(listener.ListenerArn),
				Port:     int64(awsv2.ToInt32(listener.Port)),
				Protocol: string(listener.Protocol),
			})
		}
		if output.NextMarker == nil {
			break
		}
		in.Marker = output.NextMarker
	}
	return listeners, nil
}
