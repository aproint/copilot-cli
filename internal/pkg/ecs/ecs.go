// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package ecs provides a client to retrieve Copilot ECS information.
package ecs

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/aproint/copilot-cli/internal/pkg/aws/ecs"
	"github.com/aproint/copilot-cli/internal/pkg/aws/resourcegroups"
	"github.com/aproint/copilot-cli/internal/pkg/aws/stepfunctions"
	"github.com/aproint/copilot-cli/internal/pkg/deploy"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
)

const (
	fmtWorkloadTaskDefinitionFamily = "%s-%s-%s"
	fmtTaskTaskDefinitionFamily     = "copilot-%s"
	clusterResourceType             = "ecs:cluster"
	serviceResourceType             = "ecs:service"

	taskStopReason = "Task stopped because the underlying CloudFormation stack was deleted."
)

type resourceGetter interface {
	GetResourcesByTags(resourceType string, tags map[string]string) ([]*resourcegroups.Resource, error)
	GetResourcesByTagsWithContext(ctx context.Context, resourceType string, tags map[string]string) ([]*resourcegroups.Resource, error)
}

type ecsClient interface {
	DefaultCluster() (string, error)
	DefaultClusterWithContext(ctx context.Context) (string, error)
	Service(clusterName, serviceName string) (*ecs.Service, error)
	ServiceWithContext(ctx context.Context, clusterName, serviceName string) (*ecs.Service, error)
	NetworkConfiguration(cluster, serviceName string) (*ecs.NetworkConfiguration, error)
	NetworkConfigurationWithContext(ctx context.Context, cluster, serviceName string) (*ecs.NetworkConfiguration, error)
	RunningTasks(cluster string) ([]*ecs.Task, error)
	RunningTasksWithContext(ctx context.Context, cluster string) ([]*ecs.Task, error)
	RunningTasksInFamily(cluster, family string) ([]*ecs.Task, error)
	RunningTasksInFamilyWithContext(ctx context.Context, cluster, family string) ([]*ecs.Task, error)
	ServiceRunningTasks(clusterName, serviceName string) ([]*ecs.Task, error)
	ServiceRunningTasksWithContext(ctx context.Context, clusterName, serviceName string) ([]*ecs.Task, error)
	StoppedServiceTasks(cluster, service string) ([]*ecs.Task, error)
	StoppedServiceTasksWithContext(ctx context.Context, cluster, service string) ([]*ecs.Task, error)
	StopTasks(tasks []string, opts ...ecs.StopTasksOpts) error
	StopTasksWithContext(ctx context.Context, tasks []string, opts ...ecs.StopTasksOpts) error
	TaskDefinition(taskDefName string) (*ecs.TaskDefinition, error)
	TaskDefinitionWithContext(ctx context.Context, taskDefName string) (*ecs.TaskDefinition, error)
	UpdateService(clusterName, serviceName string, opts ...ecs.UpdateServiceOpts) error
	UpdateServiceWithContext(ctx context.Context, clusterName, serviceName string, opts ...ecs.UpdateServiceOpts) error
	DescribeTasks(cluster string, taskARNs []string) ([]*ecs.Task, error)
	DescribeTasksWithContext(ctx context.Context, cluster string, taskARNs []string) ([]*ecs.Task, error)
	ActiveClusters(arns ...string) ([]string, error)
	ActiveClustersWithContext(ctx context.Context, arns ...string) ([]string, error)
	ActiveServices(clusterName string, serviceARNs ...string) ([]string, error)
	ActiveServicesWithContext(ctx context.Context, clusterName string, serviceARNs ...string) ([]string, error)
	ListServicesByNamespace(namespace string) ([]string, error)
	ListServicesByNamespaceWithContext(ctx context.Context, namespace string) ([]string, error)
	Services(cluster string, services ...string) ([]*ecs.Service, error)
	ServicesWithContext(ctx context.Context, cluster string, services ...string) ([]*ecs.Service, error)
}

type stepFunctionsClient interface {
	StateMachineDefinition(stateMachineARN string) (string, error)
	StateMachineDefinitionWithContext(ctx context.Context, stateMachineARN string) (string, error)
}

// EnvVar contains the value of an environment variable
type EnvVar struct {
	Name  string
	Value string
}

// ServiceDesc contains the description of an ECS service.
type ServiceDesc struct {
	Name         string
	ClusterName  string
	Tasks        []*ecs.Task // Tasks is a list of tasks with DesiredStatus being RUNNING.
	StoppedTasks []*ecs.Task
}

// Client retrieves Copilot information from ECS endpoint.
type Client struct {
	rgGetter       resourceGetter
	ecsClient      ecsClient
	StepFuncClient stepFunctionsClient
}

// New creates a new Client.
func New(rgConfig aws.Config) *Client {
	return &Client{
		rgGetter:  resourcegroups.New(rgConfig),
		ecsClient: ecs.New(rgConfig),
	}
}

// NewWithStepFunctionsConfig creates a new Client with a Step Functions client configured from SDK v2 config.
func NewWithStepFunctionsConfig(cfg aws.Config) *Client {
	client := New(cfg)
	client.StepFuncClient = stepfunctions.New(cfg)
	return client
}

// ClusterARN returns the ARN of the cluster in an environment.
func (c Client) ClusterARN(app, env string) (string, error) {
	return c.clusterARN(app, env)
}

// ClusterARNWithContext returns the ARN of the cluster in an environment using ctx.
func (c Client) ClusterARNWithContext(ctx context.Context, app, env string) (string, error) {
	return c.clusterARNWithContext(ctx, app, env)
}

// ForceUpdateService forces a new update for an ECS service given Copilot service info.
func (c Client) ForceUpdateService(app, env, svc string) error {
	clusterName, serviceName, err := c.fetchAndParseServiceARN(app, env, svc)
	if err != nil {
		return err
	}
	return c.ecsClient.UpdateService(clusterName, serviceName, ecs.WithForceUpdate())
}

// ForceUpdateServiceWithContext forces a service update and waits using ctx.
func (c Client) ForceUpdateServiceWithContext(ctx context.Context, app, env, svc string) error {
	clusterName, serviceName, err := c.fetchAndParseServiceARNWithContext(ctx, app, env, svc)
	if err != nil {
		return err
	}
	return c.ecsClient.UpdateServiceWithContext(ctx, clusterName, serviceName, ecs.WithForceUpdate())
}

// DescribeService returns the description of an ECS service given Copilot service info.
func (c Client) DescribeService(app, env, svc string) (*ServiceDesc, error) {
	clusterName, serviceName, err := c.fetchAndParseServiceARN(app, env, svc)
	if err != nil {
		return nil, err
	}
	tasks, err := c.ecsClient.ServiceRunningTasks(clusterName, serviceName)
	if err != nil {
		return nil, fmt.Errorf("get tasks for service %s: %w", serviceName, err)
	}
	stoppedTasks, err := c.ecsClient.StoppedServiceTasks(clusterName, serviceName)
	if err != nil {
		return nil, fmt.Errorf("get stopped tasks for service %s: %w", serviceName, err)
	}

	return &ServiceDesc{
		ClusterName:  clusterName,
		Name:         serviceName,
		Tasks:        tasks,
		StoppedTasks: stoppedTasks,
	}, nil
}

// DescribeServiceWithContext returns the description of an ECS service using ctx.
func (c Client) DescribeServiceWithContext(ctx context.Context, app, env, svc string) (*ServiceDesc, error) {
	clusterName, serviceName, err := c.fetchAndParseServiceARNWithContext(ctx, app, env, svc)
	if err != nil {
		return nil, err
	}
	tasks, err := c.ecsClient.ServiceRunningTasksWithContext(ctx, clusterName, serviceName)
	if err != nil {
		return nil, fmt.Errorf("get tasks for service %s: %w", serviceName, err)
	}
	stoppedTasks, err := c.ecsClient.StoppedServiceTasksWithContext(ctx, clusterName, serviceName)
	if err != nil {
		return nil, fmt.Errorf("get stopped tasks for service %s: %w", serviceName, err)
	}

	return &ServiceDesc{
		ClusterName:  clusterName,
		Name:         serviceName,
		Tasks:        tasks,
		StoppedTasks: stoppedTasks,
	}, nil
}

// Service returns an ECS service given Copilot service info.
func (c Client) Service(app, env, svc string) (*ecs.Service, error) {
	clusterName, serviceName, err := c.fetchAndParseServiceARN(app, env, svc)
	if err != nil {
		return nil, err
	}
	service, err := c.ecsClient.Service(clusterName, serviceName)
	if err != nil {
		return nil, fmt.Errorf("get ECS service %s: %w", serviceName, err)
	}
	return service, nil
}

// ServiceWithContext returns an ECS service using ctx.
func (c Client) ServiceWithContext(ctx context.Context, app, env, svc string) (*ecs.Service, error) {
	clusterName, serviceName, err := c.fetchAndParseServiceARNWithContext(ctx, app, env, svc)
	if err != nil {
		return nil, err
	}
	service, err := c.ecsClient.ServiceWithContext(ctx, clusterName, serviceName)
	if err != nil {
		return nil, fmt.Errorf("get ECS service %s: %w", serviceName, err)
	}
	return service, nil
}

// ServiceConnectServices returns a list of services that are in the same
// service connect namespace as the given service, except for itself.
func (c Client) ServiceConnectServices(app, env, svc string) ([]*ecs.Service, error) {
	return c.ServiceConnectServicesWithContext(context.Background(), app, env, svc)
}

// ServiceConnectServicesWithContext returns services in the same Service Connect namespace using ctx.
func (c Client) ServiceConnectServicesWithContext(ctx context.Context, app, env, svc string) ([]*ecs.Service, error) {
	s, err := c.ServiceWithContext(ctx, app, env, svc)
	if err != nil {
		return nil, fmt.Errorf("get service: %w", err)
	}
	if len(s.Deployments) == 0 || s.Deployments[0].ServiceConnectConfiguration == nil {
		return nil, nil
	}

	arns, err := c.ecsClient.ListServicesByNamespaceWithContext(ctx, aws.ToString(s.Deployments[0].ServiceConnectConfiguration.Namespace))
	if err != nil {
		return nil, fmt.Errorf("get services in the same namespace: %w", err)
	}

	// remove this service's arn
	arns = slices.DeleteFunc(arns, func(arn string) bool {
		return arn == aws.ToString(s.ServiceArn)
	})

	svcs, err := c.ecsClient.ServicesWithContext(ctx, aws.ToString(s.ClusterArn), arns...)
	if err != nil {
		return nil, fmt.Errorf("get services: %w", err)
	}
	return svcs, nil
}

// LastUpdatedAt returns the last updated time of the ECS service.
func (c Client) LastUpdatedAt(app, env, svc string) (time.Time, error) {
	detail, err := c.Service(app, env, svc)
	if err != nil {
		return time.Time{}, err
	}
	return detail.LastUpdatedAt(), nil
}

// LastUpdatedAtWithContext returns the last service update time using ctx.
func (c Client) LastUpdatedAtWithContext(ctx context.Context, app, env, svc string) (time.Time, error) {
	detail, err := c.ServiceWithContext(ctx, app, env, svc)
	if err != nil {
		return time.Time{}, err
	}
	return detail.LastUpdatedAt(), nil
}

// ListActiveAppEnvTasksOpts contains the parameters for ListActiveAppEnvTasks.
type ListActiveAppEnvTasksOpts struct {
	App string
	Env string
	ListTasksFilter
}

// ListTasksFilter contains the filtering parameters for listing Copilot tasks.
type ListTasksFilter struct {
	TaskGroup   string // Returns only tasks with the given TaskGroup name.
	TaskID      string // Returns only tasks with the given ID.
	CopilotOnly bool   // Returns only tasks with the `copilot-task` tag.
}

type listActiveCopilotTasksOpts struct {
	Cluster string
	ListTasksFilter
}

// ListActiveAppEnvTasks returns the active Copilot tasks in the environment of an application.
func (c Client) ListActiveAppEnvTasks(opts ListActiveAppEnvTasksOpts) ([]*ecs.Task, error) {
	return c.ListActiveAppEnvTasksWithContext(context.Background(), opts)
}

// ListActiveAppEnvTasksWithContext returns active Copilot tasks in an environment using ctx.
func (c Client) ListActiveAppEnvTasksWithContext(ctx context.Context, opts ListActiveAppEnvTasksOpts) ([]*ecs.Task, error) {
	clusterARN, err := c.ClusterARNWithContext(ctx, opts.App, opts.Env)
	if err != nil {
		return nil, err
	}
	return c.listActiveCopilotTasks(ctx, listActiveCopilotTasksOpts{
		Cluster:         clusterARN,
		ListTasksFilter: opts.ListTasksFilter,
	})
}

// ListActiveDefaultClusterTasks returns the active Copilot tasks in the default cluster.
func (c Client) ListActiveDefaultClusterTasks(filter ListTasksFilter) ([]*ecs.Task, error) {
	return c.ListActiveDefaultClusterTasksWithContext(context.Background(), filter)
}

// ListActiveDefaultClusterTasksWithContext returns active Copilot tasks in the default cluster using ctx.
func (c Client) ListActiveDefaultClusterTasksWithContext(ctx context.Context, filter ListTasksFilter) ([]*ecs.Task, error) {
	defaultCluster, err := c.ecsClient.DefaultClusterWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("get default cluster: %w", err)
	}
	return c.listActiveCopilotTasks(ctx, listActiveCopilotTasksOpts{
		Cluster:         defaultCluster,
		ListTasksFilter: filter,
	})
}

// StopWorkloadTasks stops all tasks in the given application, enviornment, and workload.
func (c Client) StopWorkloadTasks(app, env, workload string) error {
	return c.StopWorkloadTasksWithContext(context.Background(), app, env, workload)
}

// StopWorkloadTasksWithContext stops workload tasks using ctx.
func (c Client) StopWorkloadTasksWithContext(ctx context.Context, app, env, workload string) error {
	return c.stopTasks(ctx, app, env, ListTasksFilter{
		TaskGroup: fmt.Sprintf(fmtWorkloadTaskDefinitionFamily, app, env, workload),
	})
}

// StopOneOffTasks stops all one-off tasks in the given application and environment with the family name.
func (c Client) StopOneOffTasks(app, env, family string) error {
	return c.StopOneOffTasksWithContext(context.Background(), app, env, family)
}

// StopOneOffTasksWithContext stops one-off tasks using ctx.
func (c Client) StopOneOffTasksWithContext(ctx context.Context, app, env, family string) error {
	return c.stopTasks(ctx, app, env, ListTasksFilter{
		TaskGroup:   fmt.Sprintf(fmtTaskTaskDefinitionFamily, family),
		CopilotOnly: true,
	})
}

// stopTasks stops all tasks in the given application and environment in the given family.
func (c Client) stopTasks(ctx context.Context, app, env string, filter ListTasksFilter) error {
	tasks, err := c.ListActiveAppEnvTasksWithContext(ctx, ListActiveAppEnvTasksOpts{
		App:             app,
		Env:             env,
		ListTasksFilter: filter,
	})
	if err != nil {
		return err
	}
	taskIDs := make([]string, len(tasks))
	for n, task := range tasks {
		taskIDs[n] = aws.ToString(task.TaskArn)
	}
	clusterARN, err := c.ClusterARNWithContext(ctx, app, env)
	if err != nil {
		return fmt.Errorf("get cluster for env %s: %w", env, err)
	}
	return c.ecsClient.StopTasksWithContext(ctx, taskIDs, ecs.WithStopTaskCluster(clusterARN), ecs.WithStopTaskReason(taskStopReason))
}

// StopDefaultClusterTasks stops all copilot tasks from the given family in the default cluster.
func (c Client) StopDefaultClusterTasks(familyName string) error {
	return c.StopDefaultClusterTasksWithContext(context.Background(), familyName)
}

// StopDefaultClusterTasksWithContext stops one-off tasks in the default cluster using ctx.
func (c Client) StopDefaultClusterTasksWithContext(ctx context.Context, familyName string) error {
	tdFamily := fmt.Sprintf(fmtTaskTaskDefinitionFamily, familyName)
	tasks, err := c.ListActiveDefaultClusterTasksWithContext(ctx, ListTasksFilter{
		TaskGroup:   tdFamily,
		CopilotOnly: true,
	})
	if err != nil {
		return err
	}
	taskIDs := make([]string, len(tasks))
	for n, task := range tasks {
		taskIDs[n] = aws.ToString(task.TaskArn)
	}
	return c.ecsClient.StopTasksWithContext(ctx, taskIDs, ecs.WithStopTaskReason(taskStopReason))
}

// TaskDefinition returns the task definition of the service.
func (c Client) TaskDefinition(app, env, svc string) (*ecs.TaskDefinition, error) {
	taskDefName := fmt.Sprintf("%s-%s-%s", app, env, svc)
	taskDefinition, err := c.ecsClient.TaskDefinition(taskDefName)
	if err != nil {
		return nil, fmt.Errorf("get task definition %s of service %s: %w", taskDefName, svc, err)
	}
	return taskDefinition, nil
}

// TaskDefinitionWithContext returns the task definition of the service using ctx.
func (c Client) TaskDefinitionWithContext(ctx context.Context, app, env, svc string) (*ecs.TaskDefinition, error) {
	taskDefName := fmt.Sprintf("%s-%s-%s", app, env, svc)
	taskDefinition, err := c.ecsClient.TaskDefinitionWithContext(ctx, taskDefName)
	if err != nil {
		return nil, fmt.Errorf("get task definition %s of service %s: %w", taskDefName, svc, err)
	}
	return taskDefinition, nil
}

// NetworkConfiguration returns the network configuration of the service.
func (c Client) NetworkConfiguration(app, env, svc string) (*ecs.NetworkConfiguration, error) {
	return c.NetworkConfigurationWithContext(context.Background(), app, env, svc)
}

// NetworkConfigurationWithContext returns a service network configuration using ctx.
func (c Client) NetworkConfigurationWithContext(ctx context.Context, app, env, svc string) (*ecs.NetworkConfiguration, error) {
	clusterARN, err := c.clusterARNWithContext(ctx, app, env)
	if err != nil {
		return nil, err
	}
	arn, err := c.serviceARNWithContext(ctx, app, env, svc)
	if err != nil {
		return nil, err
	}
	return c.ecsClient.NetworkConfigurationWithContext(ctx, clusterARN, arn.ServiceName())
}

// NetworkConfigurationForJob returns the network configuration of the job.
func (c Client) NetworkConfigurationForJob(app, env, job string) (*ecs.NetworkConfiguration, error) {
	return c.NetworkConfigurationForJobWithContext(context.Background(), app, env, job)
}

// NetworkConfigurationForJobWithContext returns a job network configuration using ctx.
func (c Client) NetworkConfigurationForJobWithContext(ctx context.Context, app, env, job string) (*ecs.NetworkConfiguration, error) {
	jobARN, err := c.stateMachineARNWithContext(ctx, app, env, job)
	if err != nil {
		return nil, err
	}

	raw, err := c.StepFuncClient.StateMachineDefinitionWithContext(ctx, jobARN)
	if err != nil {
		return nil, fmt.Errorf("get state machine definition for job %s: %w", job, err)
	}

	var config NetworkConfiguration
	err = json.Unmarshal([]byte(raw), &config)
	if err != nil {
		return nil, fmt.Errorf("unmarshal state machine definition: %w", err)
	}

	return (*ecs.NetworkConfiguration)(&config), nil
}

// NetworkConfiguration wraps an ecs.NetworkConfiguration struct.
type NetworkConfiguration ecs.NetworkConfiguration

// UnmarshalJSON implements custom logic to unmarshal only the network configuration from a state machine definition.
// Example state machine definition:
//
//	 "Version": "1.0",
//	 "Comment": "Run AWS Fargate task",
//	 "StartAt": "Run Fargate Task",
//	 "States": {
//	   "Run Fargate Task": {
//		 "Type": "Task",
//		 "Resource": "arn:aws:states:::ecs:runTask.sync",
//		 "Parameters": {
//		   "LaunchType": "FARGATE",
//		   "PlatformVersion": "1.4.0",
//		   "Cluster": "cluster",
//		   "TaskDefinition": "def",
//		   "PropagateTags": "TASK_DEFINITION",
//		   "Group.$": "$$.Execution.Name",
//		   "NetworkConfiguration": {
//			 "AwsvpcConfiguration": {
//			   "Subnets": ["sbn-1", "sbn-2"],
//			   "AssignPublicIp": "ENABLED",
//			   "SecurityGroups": ["sg-1", "sg-2"]
//			 }
//		   }
//		 },
//		 "End": true
//	   }
func (n *NetworkConfiguration) UnmarshalJSON(b []byte) error {
	var f interface{}
	err := json.Unmarshal(b, &f)
	if err != nil {
		return err
	}

	states := f.(map[string]interface{})["States"].(map[string]interface{})
	parameters := states["Run Fargate Task"].(map[string]interface{})["Parameters"].(map[string]interface{})
	networkConfig := parameters["NetworkConfiguration"].(map[string]interface{})["AwsvpcConfiguration"].(map[string]interface{})

	var subnets []string
	for _, subnet := range networkConfig["Subnets"].([]interface{}) {
		subnets = append(subnets, subnet.(string))
	}

	var securityGroups []string
	for _, sg := range networkConfig["SecurityGroups"].([]interface{}) {
		securityGroups = append(securityGroups, sg.(string))
	}

	n.Subnets = subnets
	n.SecurityGroups = securityGroups
	n.AssignPublicIp = networkConfig["AssignPublicIp"].(string)
	return nil
}

func (c Client) listActiveCopilotTasks(ctx context.Context, opts listActiveCopilotTasksOpts) ([]*ecs.Task, error) {
	var tasks []*ecs.Task
	if opts.TaskGroup != "" {
		resp, err := c.ecsClient.RunningTasksInFamilyWithContext(ctx, opts.Cluster, opts.TaskGroup)
		if err != nil {
			return nil, fmt.Errorf("list running tasks in family %s and cluster %s: %w", opts.TaskGroup, opts.Cluster, err)
		}
		tasks = resp
	} else {
		resp, err := c.ecsClient.RunningTasksWithContext(ctx, opts.Cluster)
		if err != nil {
			return nil, fmt.Errorf("list running tasks in cluster %s: %w", opts.Cluster, err)
		}
		tasks = resp
	}
	if opts.CopilotOnly {
		return filterCopilotTasks(tasks, opts.TaskID), nil
	}
	return filterTasksByID(tasks, opts.TaskID), nil
}

func filterTasksByID(tasks []*ecs.Task, taskID string) []*ecs.Task {
	var filteredTasks []*ecs.Task
	for _, task := range tasks {
		id, _ := ecs.TaskID(aws.ToString(task.TaskArn))
		if strings.Contains(id, taskID) {
			filteredTasks = append(filteredTasks, task)
		}
	}
	return filteredTasks
}

func filterCopilotTasks(tasks []*ecs.Task, taskID string) []*ecs.Task {
	var filteredTasks []*ecs.Task

	for _, task := range filterTasksByID(tasks, taskID) {
		var copilotTask bool
		for _, tag := range task.Tags {
			if aws.ToString(tag.Key) == deploy.TaskTagKey {
				copilotTask = true
				break
			}
		}
		if copilotTask {
			filteredTasks = append(filteredTasks, task)
		}
	}
	return filteredTasks
}

func (c Client) clusterARN(app, env string) (string, error) {
	tags := tags(map[string]string{
		deploy.AppTagKey: app,
		deploy.EnvTagKey: env,
	})

	clusters, err := c.rgGetter.GetResourcesByTags(clusterResourceType, tags)
	switch {
	case err != nil:
		return "", fmt.Errorf("get ECS cluster with tags %s: %w", tags.String(), err)
	case len(clusters) == 0:
		return "", fmt.Errorf("no ECS cluster found with tags %s", tags.String())
	}

	arns := make([]string, len(clusters))
	for i := range clusters {
		arns[i] = clusters[i].ARN
	}

	active, err := c.ecsClient.ActiveClusters(arns...)
	switch {
	case err != nil:
		return "", fmt.Errorf("check if clusters are active: %w", err)
	case len(active) > 1:
		return "", fmt.Errorf("more than one active ECS cluster are found with tags %s", tags.String())
	}

	return active[0], nil
}

func (c Client) clusterARNWithContext(ctx context.Context, app, env string) (string, error) {
	tags := tags(map[string]string{
		deploy.AppTagKey: app,
		deploy.EnvTagKey: env,
	})

	clusters, err := c.rgGetter.GetResourcesByTagsWithContext(ctx, clusterResourceType, tags)
	switch {
	case err != nil:
		return "", fmt.Errorf("get ECS cluster with tags %s: %w", tags.String(), err)
	case len(clusters) == 0:
		return "", fmt.Errorf("no ECS cluster found with tags %s", tags.String())
	}

	arns := make([]string, len(clusters))
	for i := range clusters {
		arns[i] = clusters[i].ARN
	}

	active, err := c.ecsClient.ActiveClustersWithContext(ctx, arns...)
	switch {
	case err != nil:
		return "", fmt.Errorf("check if clusters are active: %w", err)
	case len(active) > 1:
		return "", fmt.Errorf("more than one active ECS cluster are found with tags %s", tags.String())
	}

	return active[0], nil
}

func (c Client) fetchAndParseServiceARN(app, env, svc string) (cluster, service string, err error) {
	svcARN, err := c.serviceARN(app, env, svc)
	if err != nil {
		return "", "", err
	}
	return svcARN.ClusterName(), svcARN.ServiceName(), nil
}

func (c Client) fetchAndParseServiceARNWithContext(ctx context.Context, app, env, svc string) (cluster, service string, err error) {
	svcARN, err := c.serviceARNWithContext(ctx, app, env, svc)
	if err != nil {
		return "", "", err
	}
	return svcARN.ClusterName(), svcARN.ServiceName(), nil
}

func (c Client) serviceARN(app, env, svc string) (*ecs.ServiceArn, error) {
	tags := tags(map[string]string{
		deploy.AppTagKey:     app,
		deploy.EnvTagKey:     env,
		deploy.ServiceTagKey: svc,
	})
	services, err := c.rgGetter.GetResourcesByTags(serviceResourceType, tags)
	if err != nil {
		return nil, fmt.Errorf("get ECS service with tags %s: %w", tags.String(), err)
	}
	if len(services) == 0 {
		return nil, fmt.Errorf("no ECS service found with tags %s", tags.String())
	}
	arns := make([]string, len(services))
	for i := range services {
		arns[i] = services[i].ARN
	}
	activeCluster, err := c.clusterARN(app, env)
	if err != nil {
		return nil, err
	}
	activeSvcs, err := c.ecsClient.ActiveServices(activeCluster, arns...)
	if err != nil {
		return nil, fmt.Errorf("check if services are active in the cluster %s: %w", activeCluster, err)
	}
	if len(activeSvcs) > 1 {
		return nil, fmt.Errorf("more than one ECS service with tags %s", tags.String())
	}
	if len(activeSvcs) == 0 {
		return nil, fmt.Errorf("no active ECS service found")
	}
	serviceARN, err := ecs.ParseServiceArn(activeSvcs[0])
	if err != nil {
		return nil, fmt.Errorf("parse service arn: %w", err)
	}
	return serviceARN, nil
}

func (c Client) serviceARNWithContext(ctx context.Context, app, env, svc string) (*ecs.ServiceArn, error) {
	tags := tags(map[string]string{
		deploy.AppTagKey:     app,
		deploy.EnvTagKey:     env,
		deploy.ServiceTagKey: svc,
	})
	services, err := c.rgGetter.GetResourcesByTagsWithContext(ctx, serviceResourceType, tags)
	if err != nil {
		return nil, fmt.Errorf("get ECS service with tags %s: %w", tags.String(), err)
	}
	if len(services) == 0 {
		return nil, fmt.Errorf("no ECS service found with tags %s", tags.String())
	}
	arns := make([]string, len(services))
	for i := range services {
		arns[i] = services[i].ARN
	}
	activeCluster, err := c.clusterARNWithContext(ctx, app, env)
	if err != nil {
		return nil, err
	}
	activeSvcs, err := c.ecsClient.ActiveServicesWithContext(ctx, activeCluster, arns...)
	if err != nil {
		return nil, fmt.Errorf("check if services are active in the cluster %s: %w", activeCluster, err)
	}
	if len(activeSvcs) == 0 {
		return nil, fmt.Errorf("no active ECS service found")
	}
	if len(activeSvcs) > 1 {
		return nil, fmt.Errorf("more than one ECS service with tags %s", tags.String())
	}
	serviceARN, err := ecs.ParseServiceArn(activeSvcs[0])
	if err != nil {
		return nil, fmt.Errorf("parse service arn: %w", err)
	}
	return serviceARN, nil
}

type tags map[string]string

func (tags tags) String() string {
	serialized := make([]string, len(tags))
	var i = 0
	for k, v := range tags {
		serialized[i] = fmt.Sprintf("%q=%q", k, v)
		i += 1
	}
	sort.SliceStable(serialized, func(i, j int) bool { return serialized[i] < serialized[j] })
	return strings.Join(serialized, ",")
}

func (c Client) stateMachineARN(app, env, job string) (string, error) {
	return c.stateMachineARNWithContext(context.Background(), app, env, job)
}

func (c Client) stateMachineARNWithContext(ctx context.Context, app, env, job string) (string, error) {
	tags := tags(map[string]string{
		deploy.AppTagKey:     app,
		deploy.EnvTagKey:     env,
		deploy.ServiceTagKey: job,
	})
	resources, err := c.rgGetter.GetResourcesByTagsWithContext(ctx, resourcegroups.ResourceTypeStateMachine, tags)
	if err != nil {
		return "", fmt.Errorf("get state machine resource with tags %s: %w", tags.String(), err)
	}

	var stateMachineARN string
	targetName := fmt.Sprintf(fmtStateMachineName, app, env, job)
	for _, r := range resources {
		parsedARN, err := arn.Parse(r.ARN)
		if err != nil {
			continue
		}
		parts := strings.Split(parsedARN.Resource, ":")
		if len(parts) != 2 {
			continue
		}
		if parts[1] == targetName {
			stateMachineARN = r.ARN
			break
		}
	}

	if stateMachineARN == "" {
		return "", fmt.Errorf("state machine for job %s not found", job)
	}
	return stateMachineARN, nil
}

// HasNonZeroExitCode returns an error if at least one of the tasks exited with a non-zero exit code. It assumes that all tasks are built on the same task definition.
func (c Client) HasNonZeroExitCode(taskARNs []string, cluster string) error {
	return c.HasNonZeroExitCodeWithContext(context.Background(), taskARNs, cluster)
}

// HasNonZeroExitCodeWithContext checks task exit codes using ctx.
func (c Client) HasNonZeroExitCodeWithContext(ctx context.Context, taskARNs []string, cluster string) error {
	tasks, err := c.ecsClient.DescribeTasksWithContext(ctx, cluster, taskARNs)
	if err != nil {
		return fmt.Errorf("describe tasks %s: %w", taskARNs, err)
	}

	if len(tasks) == 0 {
		return fmt.Errorf("cannot find tasks %s", strings.Join(taskARNs, ", "))
	}

	taskDefinitonARN := aws.ToString(tasks[0].TaskDefinitionArn)
	taskDefinition, err := c.ecsClient.TaskDefinitionWithContext(ctx, taskDefinitonARN)
	if err != nil {
		return fmt.Errorf("get task definition %s: %w", taskDefinitonARN, err)
	}

	isContainerEssential := make(map[string]bool)
	for _, container := range taskDefinition.ContainerDefinitions {
		isContainerEssential[aws.ToString(container.Name)] = aws.ToBool(container.Essential)
	}

	for _, describedTask := range tasks {
		for _, container := range describedTask.Containers {
			if isContainerEssential[aws.ToString(container.Name)] && aws.ToInt32(container.ExitCode) != 0 {
				taskID, err := ecs.TaskID(aws.ToString(describedTask.TaskArn))
				if err != nil {
					return err
				}
				return &ErrExitCode{aws.ToString(container.Name),
					taskID,
					int(aws.ToInt32(container.ExitCode))}
			}
		}
	}
	return nil
}
