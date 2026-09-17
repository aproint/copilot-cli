// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// Copyright APROINT, s.r.o. in modifications to this fork.
// SPDX-License-Identifier: Apache-2.0

// Package logging contains utility functions for ECS logging.
package logging

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aproint/copilot-cli/internal/pkg/aws/cloudwatchlogs"
	"github.com/aproint/copilot-cli/internal/pkg/aws/ecs"
	"github.com/aproint/copilot-cli/internal/pkg/task"
	"github.com/aproint/copilot-cli/internal/pkg/term/log"
	"github.com/aws/aws-sdk-go-v2/aws"
)

const (
	numCWLogsCallsPerRound = 10
	fmtTaskLogGroupName    = "/copilot/%s"
	// e.g., copilot-task/python/4f8243e83f8a4bdaa7587fa1eaff2ea3
	fmtTaskLogStreamName = "copilot-task/%s/%s"
)

// TasksDescriber describes ECS tasks.
type TasksDescriber interface {
	DescribeTasks(ctx context.Context, cluster string, taskARNs []string) ([]*ecs.Task, error)
}

// TaskClient retrieves the logs of Amazon ECS tasks.
type TaskClient struct {
	// Inputs to the task client.
	groupName string
	tasks     []*task.Task

	eventsWriter  io.Writer
	eventsLogger  logGetter
	taskDescriber TasksDescriber

	// Replaced in tests.
	sleep func(context.Context) error
}

// NewTaskClient returns a TaskClient that can retrieve logs from the given tasks under the groupName.
func NewTaskClient(cfg aws.Config, groupName string, tasks []*task.Task) *TaskClient {
	return &TaskClient{
		groupName: groupName,
		tasks:     tasks,

		taskDescriber: ecs.New(cfg),
		eventsLogger:  cloudwatchlogs.New(cfg),
		eventsWriter:  log.OutputWriter,

		sleep: func(ctx context.Context) error {
			timer := time.NewTimer(cloudwatchlogs.SleepDuration)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
	}
}

// WriteEventsUntilStopped writes task events until all tasks stop or ctx is canceled.
func (t *TaskClient) WriteEventsUntilStopped(ctx context.Context) error {
	in := cloudwatchlogs.LogEventsOpts{
		LogGroup: fmt.Sprintf(fmtTaskLogGroupName, t.groupName),
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		logStreams, err := t.logStreamNamesFromTasks(t.tasks)
		if err != nil {
			return err
		}
		in.LogStreamPrefixFilters = logStreams
		for i := 0; i < numCWLogsCallsPerRound; i++ {
			logEventsOutput, err := t.eventsLogger.LogEvents(ctx, in)
			if err != nil {
				return fmt.Errorf("get task log events: %w", err)
			}
			if err := WriteHumanLogs(t.eventsWriter, cwEventsToHumanJSONStringers(logEventsOutput.Events)); err != nil {
				return fmt.Errorf("write log event: %w", err)
			}
			in.StreamLastEventTime = logEventsOutput.StreamLastEventTime

			if err := t.sleep(ctx); err != nil {
				return err
			}
		}
		stopped, err := t.allTasksStopped(ctx)
		if err != nil {
			return err
		}
		if stopped {
			return nil
		}
	}
}

func (t *TaskClient) allTasksStopped(ctx context.Context) (bool, error) {
	taskARNs := make([]string, len(t.tasks))
	for idx, task := range t.tasks {
		taskARNs[idx] = task.TaskARN
	}

	// NOTE: all tasks are deployed to the same cluster and there are at least one tasks being deployed
	cluster := t.tasks[0].ClusterARN

	tasksResp, err := t.taskDescriber.DescribeTasks(ctx, cluster, taskARNs)
	if err != nil {
		return false, fmt.Errorf("describe tasks: %w", err)
	}

	stopped := true
	var runningTasks []*task.Task
	for _, t := range tasksResp {
		if *t.LastStatus != ecs.DesiredStatusStopped {
			stopped = false
			runningTasks = append(runningTasks, &task.Task{
				ClusterARN: *t.ClusterArn,
				TaskARN:    *t.TaskArn,
			})
		}
	}
	t.tasks = runningTasks
	return stopped, nil
}

func (t *TaskClient) logStreamNamesFromTasks(tasks []*task.Task) ([]string, error) {
	var logStreamNames []string
	for _, task := range tasks {
		id, err := ecs.TaskID(task.TaskARN)
		if err != nil {
			return nil, fmt.Errorf("parse task ID from ARN %s", task.TaskARN)
		}
		logStreamNames = append(logStreamNames, fmt.Sprintf(fmtTaskLogStreamName, t.groupName, id))
	}
	return logStreamNames, nil
}
