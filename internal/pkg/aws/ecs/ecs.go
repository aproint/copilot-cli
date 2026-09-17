// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// Copyright APROINT, s.r.o. in modifications to this fork.
// SPDX-License-Identifier: Apache-2.0

// Package ecs provides a client to make API requests to Amazon Elastic Container Service.
package ecs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aproint/copilot-cli/internal/pkg/exec"
	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

const (
	statusActive                     = "ACTIVE"
	waitServiceStablePollingInterval = 15 * time.Second
	waitServiceStableMaxTry          = 80
	stableServiceDeploymentNum       = 1

	// EndpointsID is the ID to look up the ECS service endpoint.
	EndpointsID = "ecs"
)

type api interface {
	DescribeClusters(ctx context.Context, input *ecs.DescribeClustersInput, opts ...func(*ecs.Options)) (*ecs.DescribeClustersOutput, error)
	DescribeServices(ctx context.Context, input *ecs.DescribeServicesInput, opts ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error)
	DescribeTasks(ctx context.Context, input *ecs.DescribeTasksInput, opts ...func(*ecs.Options)) (*ecs.DescribeTasksOutput, error)
	DescribeTaskDefinition(ctx context.Context, input *ecs.DescribeTaskDefinitionInput, opts ...func(*ecs.Options)) (*ecs.DescribeTaskDefinitionOutput, error)
	ExecuteCommand(ctx context.Context, input *ecs.ExecuteCommandInput, opts ...func(*ecs.Options)) (*ecs.ExecuteCommandOutput, error)
	ListServicesByNamespace(ctx context.Context, input *ecs.ListServicesByNamespaceInput, opts ...func(*ecs.Options)) (*ecs.ListServicesByNamespaceOutput, error)
	ListTasks(ctx context.Context, input *ecs.ListTasksInput, opts ...func(*ecs.Options)) (*ecs.ListTasksOutput, error)
	RunTask(ctx context.Context, input *ecs.RunTaskInput, opts ...func(*ecs.Options)) (*ecs.RunTaskOutput, error)
	StopTask(ctx context.Context, input *ecs.StopTaskInput, opts ...func(*ecs.Options)) (*ecs.StopTaskOutput, error)
	UpdateService(ctx context.Context, input *ecs.UpdateServiceInput, opts ...func(*ecs.Options)) (*ecs.UpdateServiceOutput, error)
	WaitUntilTasksRunning(ctx context.Context, input *ecs.DescribeTasksInput, maxWaitDur time.Duration, opts ...func(*ecs.TasksRunningWaiterOptions)) error
}

type ssmSessionStarter interface {
	StartSession(ctx context.Context, ssmSession *types.Session) error
}

type clientWithWaiter struct {
	*ecs.Client
}

func (c clientWithWaiter) WaitUntilTasksRunning(ctx context.Context, input *ecs.DescribeTasksInput, maxWaitDur time.Duration, opts ...func(*ecs.TasksRunningWaiterOptions)) error {
	return ecs.NewTasksRunningWaiter(c.Client).Wait(ctx, input, maxWaitDur, opts...)
}

// ECS wraps an AWS ECS client.
type ECS struct {
	client         api
	newSessStarter func() ssmSessionStarter

	maxServiceStableTries int
	pollIntervalDuration  time.Duration
}

// RunTaskInput holds the fields needed to run tasks.
type RunTaskInput struct {
	Cluster         string
	Count           int
	Subnets         []string
	SecurityGroups  []string
	TaskFamilyName  string
	StartedBy       string
	PlatformVersion string
	EnableExec      bool
}

// ExecuteCommandInput holds the fields needed to execute commands in a running container.
type ExecuteCommandInput struct {
	Cluster   string
	Command   string
	Task      string
	Container string
}

// New returns a Service configured against the input config.
func New(cfg awsv2.Config) *ECS {
	client := ecs.NewFromConfig(cfg)
	return &ECS{
		client: clientWithWaiter{Client: client},
		newSessStarter: func() ssmSessionStarter {
			return exec.NewSSMPluginCommand(cfg.Region)
		},
		maxServiceStableTries: waitServiceStableMaxTry,
		pollIntervalDuration:  waitServiceStablePollingInterval,
	}
}

// TaskDefinition calls ECS API using ctx and returns the task definition.
func (e *ECS) TaskDefinition(ctx context.Context, taskDefName string) (*TaskDefinition, error) {
	resp, err := e.client.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{
		TaskDefinition: awsv2.String(taskDefName),
	})
	if err != nil {
		return nil, fmt.Errorf("describe task definition %s: %w", taskDefName, err)
	}
	td := TaskDefinition(*resp.TaskDefinition)
	return &td, nil
}

// Service calls ECS API using ctx and returns the specified service running in the cluster.
func (e *ECS) Service(ctx context.Context, clusterName, serviceName string) (*Service, error) {
	svcs, err := e.Services(ctx, clusterName, serviceName)
	if err != nil {
		return nil, err
	}
	if awsv2.ToString(svcs[0].ServiceName) != serviceName {
		return nil, fmt.Errorf("cannot find service %s", serviceName)
	}

	return svcs[0], nil
}

// Services calls ECS API using ctx and returns all specified services running in cluster.
func (e *ECS) Services(ctx context.Context, cluster string, services ...string) ([]*Service, error) {
	var svcs []*Service

	for i := 0; i < len(services); i += 10 {
		split := services[i:min(10+i, len(services))]

		resp, err := e.client.DescribeServices(ctx, &ecs.DescribeServicesInput{
			Cluster:  awsv2.String(cluster),
			Services: split,
		})
		switch {
		case err != nil:
			return nil, fmt.Errorf("describe services: %w", err)
		case len(resp.Failures) > 0:
			return nil, fmt.Errorf("describe services: %s", failureString(resp.Failures[0]))
		case len(resp.Services) != len(split):
			return nil, fmt.Errorf("describe services: got %v services, but expected %v", len(resp.Services), len(split))
		}

		for j := range resp.Services {
			svc := Service(resp.Services[j])
			svcs = append(svcs, &svc)
		}
	}

	return svcs, nil
}

// ListServicesByNamespace returns service ARNs in the namespace using ctx for every page.
func (e *ECS) ListServicesByNamespace(ctx context.Context, namespace string) ([]string, error) {
	var arns []string
	in := &ecs.ListServicesByNamespaceInput{
		Namespace: awsv2.String(namespace),
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		resp, err := e.client.ListServicesByNamespace(ctx, in)
		if err != nil {
			return nil, err
		}
		arns = append(arns, resp.ServiceArns...)
		if resp.NextToken == nil {
			break
		}
		in.NextToken = resp.NextToken
	}
	return arns, nil
}

// UpdateServiceOpts sets the optional parameter for UpdateService.
type UpdateServiceOpts func(*ecs.UpdateServiceInput)

// WithForceUpdate sets ForceNewDeployment to force an update.
func WithForceUpdate() UpdateServiceOpts {
	return func(in *ecs.UpdateServiceInput) {
		in.ForceNewDeployment = true
	}
}

// UpdateService updates a service and waits for it to become stable using ctx.
func (e *ECS) UpdateService(ctx context.Context, clusterName, serviceName string, opts ...UpdateServiceOpts) error {
	in := &ecs.UpdateServiceInput{
		Cluster: awsv2.String(clusterName),
		Service: awsv2.String(serviceName),
	}
	for _, opt := range opts {
		opt(in)
	}
	svc, err := e.client.UpdateService(ctx, in)
	if err != nil {
		return fmt.Errorf("update service %s from cluster %s: %w", serviceName, clusterName, err)
	}
	s := Service(*svc.Service)
	if err := e.waitUntilServiceStable(ctx, &s); err != nil {
		return fmt.Errorf("wait until service %s becomes stable: %w", serviceName, err)
	}
	return nil
}

// waitUntilServiceStable waits until the service is stable.
// See https://docs.aws.amazon.com/cli/latest/reference/ecs/wait/services-stable.html
func (e *ECS) waitUntilServiceStable(ctx context.Context, svc *Service) error {
	var err error
	var tryNum int
	for {
		if len(svc.Deployments) == stableServiceDeploymentNum &&
			svc.DesiredCount == svc.RunningCount {
			// This conditional is sufficient to determine that the service is stable AND has successfully updated (hence not rolled-back).
			// The stable service cannot be a rolled-back service because a rollback can only be triggered by circuit breaker after ~1hr,
			// by which time we would have already timed out.
			return nil
		}
		if tryNum >= e.maxServiceStableTries {
			return &ErrWaitServiceStableTimeout{
				maxRetries: e.maxServiceStableTries,
			}
		}
		svc, err = e.Service(ctx, awsv2.ToString(svc.ClusterArn), awsv2.ToString(svc.ServiceName))
		if err != nil {
			return err
		}
		tryNum++
		timer := time.NewTimer(e.pollIntervalDuration)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// ServiceRunningTasks returns running service tasks using ctx.
func (e *ECS) ServiceRunningTasks(ctx context.Context, cluster, service string) ([]*Task, error) {
	return e.listTasks(ctx, cluster, withService(service), withRunningTasks())
}

// StoppedServiceTasks returns stopped service tasks using ctx.
func (e *ECS) StoppedServiceTasks(ctx context.Context, cluster, service string) ([]*Task, error) {
	return e.listTasks(ctx, cluster, withService(service), withStoppedTasks())
}

// RunningTasksInFamily returns running tasks in a family using ctx.
func (e *ECS) RunningTasksInFamily(ctx context.Context, cluster, family string) ([]*Task, error) {
	return e.listTasks(ctx, cluster, withFamily(family), withRunningTasks())
}

// RunningTasks returns running tasks using ctx.
func (e *ECS) RunningTasks(ctx context.Context, cluster string) ([]*Task, error) {
	return e.listTasks(ctx, cluster, withRunningTasks())
}

type listTasksOpts func(*ecs.ListTasksInput)

func withService(svcName string) listTasksOpts {
	return func(in *ecs.ListTasksInput) {
		in.ServiceName = awsv2.String(svcName)
	}
}

func withFamily(family string) listTasksOpts {
	return func(in *ecs.ListTasksInput) {
		in.Family = awsv2.String(family)
	}
}

func withRunningTasks() listTasksOpts {
	return func(in *ecs.ListTasksInput) {
		in.DesiredStatus = types.DesiredStatusRunning
	}
}

func withStoppedTasks() listTasksOpts {
	return func(in *ecs.ListTasksInput) {
		in.DesiredStatus = types.DesiredStatusStopped
	}
}

func (e *ECS) listTasks(ctx context.Context, cluster string, opts ...listTasksOpts) ([]*Task, error) {
	var tasks []*Task
	in := &ecs.ListTasksInput{
		Cluster: awsv2.String(cluster),
	}
	for _, opt := range opts {
		opt(in)
	}
	for {
		listTaskResp, err := e.client.ListTasks(ctx, in)
		if err != nil {
			return nil, fmt.Errorf("list running tasks: %w", err)
		}
		if len(listTaskResp.TaskArns) == 0 {
			return tasks, nil
		}
		descTaskResp, err := e.client.DescribeTasks(ctx, &ecs.DescribeTasksInput{
			Cluster: awsv2.String(cluster),
			Tasks:   listTaskResp.TaskArns,
			Include: []types.TaskField{types.TaskFieldTags},
		})
		if err != nil {
			return nil, fmt.Errorf("describe running tasks in cluster %s: %w", cluster, err)
		}
		for _, task := range descTaskResp.Tasks {
			t := Task(task)
			tasks = append(tasks, &t)
		}
		if listTaskResp.NextToken == nil {
			break
		}
		in.NextToken = listTaskResp.NextToken
	}
	return tasks, nil
}

// StopTasksOpts sets the optional parameter for StopTasks.
type StopTasksOpts func(*ecs.StopTaskInput)

// WithStopTaskReason sets an optional message specified when a task is stopped.
func WithStopTaskReason(reason string) StopTasksOpts {
	return func(in *ecs.StopTaskInput) {
		in.Reason = awsv2.String(reason)
	}
}

// WithStopTaskCluster sets the cluster that hosts the task to stop.
func WithStopTaskCluster(cluster string) StopTasksOpts {
	return func(in *ecs.StopTaskInput) {
		in.Cluster = awsv2.String(cluster)
	}
}

// StopTasks stops multiple running tasks using ctx.
func (e *ECS) StopTasks(ctx context.Context, tasks []string, opts ...StopTasksOpts) error {
	in := &ecs.StopTaskInput{}
	for _, opt := range opts {
		opt(in)
	}
	for _, task := range tasks {
		in.Task = awsv2.String(task)
		if _, err := e.client.StopTask(ctx, in); err != nil {
			return fmt.Errorf("stop task %s: %w", task, err)
		}
	}
	return nil
}

// DefaultCluster returns the default cluster ARN using ctx.
func (e *ECS) DefaultCluster(ctx context.Context) (string, error) {
	resp, err := e.client.DescribeClusters(ctx, &ecs.DescribeClustersInput{})
	if err != nil {
		return "", fmt.Errorf("get default cluster: %w", err)
	}

	if len(resp.Clusters) == 0 {
		return "", ErrNoDefaultCluster
	}

	// NOTE: right now at most 1 default cluster is possible, so cluster[0] must be the default cluster
	cluster := resp.Clusters[0]
	if awsv2.ToString(cluster.Status) != statusActive {
		return "", ErrNoDefaultCluster
	}

	return awsv2.ToString(cluster.ClusterArn), nil
}

// HasDefaultCluster reports whether the default cluster exists using ctx.
func (e *ECS) HasDefaultCluster(ctx context.Context) (bool, error) {
	if _, err := e.DefaultCluster(ctx); err != nil {
		if errors.Is(err, ErrNoDefaultCluster) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// ActiveClusters returns active clusters using ctx.
func (e *ECS) ActiveClusters(ctx context.Context, arns ...string) ([]string, error) {
	resp, err := e.client.DescribeClusters(ctx, &ecs.DescribeClustersInput{
		Clusters: arns,
	})
	switch {
	case err != nil:
		return nil, fmt.Errorf("describe clusters: %w", err)
	case len(resp.Failures) > 0:
		return nil, fmt.Errorf("describe clusters: %s", failureString(resp.Failures[0]))
	}

	var active []string
	for _, cluster := range resp.Clusters {
		if awsv2.ToString(cluster.Status) == statusActive {
			active = append(active, awsv2.ToString(cluster.ClusterArn))
		}
	}

	return active, nil
}

// ActiveServices returns active services using ctx.
func (e *ECS) ActiveServices(ctx context.Context, clusterARN string, serviceARNs ...string) ([]string, error) {
	// All the filteredSvcARNs will belong to the given Cluster.
	filteredSvcARNS, err := e.filterServiceARNs(clusterARN, serviceARNs...)
	if err != nil {
		return nil, err
	}
	resp, err := e.client.DescribeServices(ctx, &ecs.DescribeServicesInput{
		Cluster:  awsv2.String(clusterARN),
		Services: filteredSvcARNS,
	})
	switch {
	case err != nil:
		return nil, fmt.Errorf("describe services: %w", err)
	case len(resp.Failures) > 0:
		return nil, fmt.Errorf("describe services: %s", failureString(resp.Failures[0]))
	}

	var active []string
	for _, svc := range resp.Services {
		if awsv2.ToString(svc.Status) == statusActive {
			active = append(active, awsv2.ToString(svc.ServiceArn))
		}
	}

	return active, nil
}

// RunTask runs tasks and waits for them to start using ctx.
func (e *ECS) RunTask(ctx context.Context, input RunTaskInput) ([]*Task, error) {
	resp, err := e.client.RunTask(ctx, &ecs.RunTaskInput{
		Cluster:        awsv2.String(input.Cluster),
		Count:          awsv2.Int32(int32(input.Count)),
		LaunchType:     types.LaunchTypeFargate,
		StartedBy:      awsv2.String(input.StartedBy),
		TaskDefinition: awsv2.String(input.TaskFamilyName),
		NetworkConfiguration: &types.NetworkConfiguration{
			AwsvpcConfiguration: &types.AwsVpcConfiguration{
				AssignPublicIp: types.AssignPublicIpEnabled,
				Subnets:        input.Subnets,
				SecurityGroups: input.SecurityGroups,
			},
		},
		EnableExecuteCommand: input.EnableExec,
		PlatformVersion:      awsv2.String(input.PlatformVersion),
		PropagateTags:        types.PropagateTagsTaskDefinition,
	})
	if err != nil {
		return nil, fmt.Errorf("run task(s) %s: %w", input.TaskFamilyName, err)
	}

	taskARNs := make([]string, len(resp.Tasks))
	for idx, task := range resp.Tasks {
		taskARNs[idx] = awsv2.ToString(task.TaskArn)
	}

	waitErr := e.client.WaitUntilTasksRunning(ctx, &ecs.DescribeTasksInput{
		Cluster: awsv2.String(input.Cluster),
		Tasks:   taskARNs,
		Include: []types.TaskField{types.TaskFieldTags},
	}, 10*time.Minute)

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if waitErr != nil && !isRequestTimeoutErr(waitErr) {
		return nil, fmt.Errorf("wait for tasks to be running: %w", waitErr)
	}

	tasks, describeErr := e.DescribeTasks(ctx, input.Cluster, taskARNs)
	if describeErr != nil {
		return nil, describeErr
	}

	if waitErr != nil {
		return nil, &ErrWaiterResourceNotReadyForTasks{tasks: tasks, awsErrResourceNotReady: waitErr}
	}

	return tasks, nil
}

// DescribeTasks returns tasks using ctx.
func (e *ECS) DescribeTasks(ctx context.Context, cluster string, taskARNs []string) ([]*Task, error) {
	resp, err := e.client.DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Cluster: awsv2.String(cluster),
		Tasks:   taskARNs,
		Include: []types.TaskField{types.TaskFieldTags},
	})
	if err != nil {
		return nil, fmt.Errorf("describe tasks: %w", err)
	}

	tasks := make([]*Task, len(resp.Tasks))
	for idx, task := range resp.Tasks {
		t := Task(task)
		tasks[idx] = &t
	}
	return tasks, nil
}

// ExecuteCommand executes a command and runs its interactive session using ctx.
func (e *ECS) ExecuteCommand(ctx context.Context, in ExecuteCommandInput) (err error) {
	execCmdresp, err := e.client.ExecuteCommand(ctx, &ecs.ExecuteCommandInput{
		Cluster:     awsv2.String(in.Cluster),
		Command:     awsv2.String(in.Command),
		Container:   awsv2.String(in.Container),
		Interactive: true,
		Task:        awsv2.String(in.Task),
	})
	if err != nil {
		return &ErrExecuteCommand{err: err}
	}
	sessID := awsv2.ToString(execCmdresp.Session.SessionId)
	if err = e.newSessStarter().StartSession(ctx, execCmdresp.Session); err != nil {
		err = fmt.Errorf("start session %s using ssm plugin: %w", sessID, err)
	}
	return err
}

// NetworkConfiguration returns a service network configuration using ctx.
func (e *ECS) NetworkConfiguration(ctx context.Context, cluster, serviceName string) (*NetworkConfiguration, error) {
	service, err := e.service(ctx, cluster, serviceName)
	if err != nil {
		return nil, err
	}

	networkConfig := service.NetworkConfiguration
	if networkConfig == nil || networkConfig.AwsvpcConfiguration == nil {
		return nil, fmt.Errorf("cannot find the awsvpc configuration for service %s", serviceName)
	}

	return &NetworkConfiguration{
		AssignPublicIp: string(networkConfig.AwsvpcConfiguration.AssignPublicIp),
		SecurityGroups: networkConfig.AwsvpcConfiguration.SecurityGroups,
		Subnets:        networkConfig.AwsvpcConfiguration.Subnets,
	}, nil
}

func (e *ECS) service(ctx context.Context, clusterName, serviceName string) (*Service, error) {
	resp, err := e.client.DescribeServices(ctx, &ecs.DescribeServicesInput{
		Cluster:  awsv2.String(clusterName),
		Services: []string{serviceName},
	})
	if err != nil {
		return nil, fmt.Errorf("describe service %s: %w", serviceName, err)
	}
	for _, service := range resp.Services {
		if awsv2.ToString(service.ServiceName) == serviceName {
			svc := Service(service)
			return &svc, nil
		}
	}
	return nil, fmt.Errorf("cannot find service %s", serviceName)
}

// filterServiceARNs returns subset of the ServiceARNs that belong to the given Cluster.
func (e *ECS) filterServiceARNs(clusterARN string, serviceARNs ...string) ([]string, error) {
	var filtered []string
	for _, arn := range serviceARNs {
		svcArn, err := ParseServiceArn(arn)
		if err != nil {
			return nil, err
		}
		if svcArn.ClusterArn() == clusterARN {
			filtered = append(filtered, arn)
		}
	}
	return filtered, nil
}

func isRequestTimeoutErr(err error) bool {
	return strings.Contains(err.Error(), "exceeded max wait time for TasksRunning waiter")
}

func failureString(f types.Failure) string {
	var fields []string
	if f.Arn != nil {
		fields = append(fields, fmt.Sprintf("  Arn: %q", awsv2.ToString(f.Arn)))
	}
	if f.Detail != nil {
		fields = append(fields, fmt.Sprintf("  Detail: %q", awsv2.ToString(f.Detail)))
	}
	if f.Reason != nil {
		fields = append(fields, fmt.Sprintf("  Reason: %q", awsv2.ToString(f.Reason)))
	}
	if len(fields) == 0 {
		return "{}"
	}
	return fmt.Sprintf("{\n%s\n}", strings.Join(fields, ",\n"))
}
