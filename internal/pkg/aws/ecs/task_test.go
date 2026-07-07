// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package ecs

import (
	"errors"
	"fmt"
	"testing"
	"time"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func TestTask_TaskStatus(t *testing.T) {
	startTime, _ := time.Parse(time.RFC3339, "2006-01-02T15:04:05+00:00")
	stopTime, _ := time.Parse(time.RFC3339, "2006-01-02T16:04:05+00:00")
	mockImageDigest := "18f7eb6cff6e63e5f5273fb53f672975fe6044580f66c354f55d2de8dd28aec7"
	testCases := map[string]struct {
		health        types.HealthStatus
		taskArn       *string
		containers    []types.Container
		lastStatus    *string
		startedAt     time.Time
		stoppedAt     time.Time
		stoppedReason *string

		wantTaskStatus *TaskStatus
		wantErr        error
	}{
		"errors if failed to parse task ID": {
			taskArn: awsv2.String("badTaskArn"),
			wantErr: fmt.Errorf("parse ECS task ARN: arn: invalid prefix"),
		},
		"success with a provisioning task": {
			taskArn: awsv2.String("arn:aws:ecs:us-west-2:123456789:task/my-project-test-Cluster-9F7Y0RLP60R7/4082490ee6c245e09d2145010aa1ba8d"),
			containers: []types.Container{
				{
					Image:       awsv2.String("mockImageArn"),
					ImageDigest: awsv2.String("sha256:" + mockImageDigest),
				},
			},
			health:     types.HealthStatusHealthy,
			lastStatus: awsv2.String("UNKNOWN"),

			wantTaskStatus: &TaskStatus{
				Health: "HEALTHY",
				ID:     "4082490ee6c245e09d2145010aa1ba8d",
				Images: []Image{
					{
						Digest: mockImageDigest,
						ID:     "mockImageArn",
					},
				},
				LastStatus: "UNKNOWN",
			},
		},
		"success with a running task": {
			taskArn: awsv2.String("arn:aws:ecs:us-west-2:123456789:task/my-project-test-Cluster-9F7Y0RLP60R7/4082490ee6c245e09d2145010aa1ba8d"),
			containers: []types.Container{
				{
					Image:       awsv2.String("mockImageArn"),
					ImageDigest: awsv2.String("sha256:" + mockImageDigest),
				},
			},
			health:     types.HealthStatusHealthy,
			lastStatus: awsv2.String("UNKNOWN"),
			startedAt:  startTime,

			wantTaskStatus: &TaskStatus{
				Health: "HEALTHY",
				ID:     "4082490ee6c245e09d2145010aa1ba8d",
				Images: []Image{
					{
						Digest: mockImageDigest,
						ID:     "mockImageArn",
					},
				},
				LastStatus: "UNKNOWN",
				StartedAt:  startTime,
			},
		},
		"success with a stopped task": {
			taskArn: awsv2.String("arn:aws:ecs:us-west-2:123456789:task/my-project-test-Cluster-9F7Y0RLP60R7/4082490ee6c245e09d2145010aa1ba8d"),
			containers: []types.Container{
				{
					Image:       awsv2.String("mockImageArn"),
					ImageDigest: awsv2.String("sha256:" + mockImageDigest),
				},
			},
			health:        types.HealthStatusHealthy,
			lastStatus:    awsv2.String("UNKNOWN"),
			startedAt:     startTime,
			stoppedAt:     stopTime,
			stoppedReason: awsv2.String("some reason"),

			wantTaskStatus: &TaskStatus{
				Health: "HEALTHY",
				ID:     "4082490ee6c245e09d2145010aa1ba8d",
				Images: []Image{
					{
						Digest: mockImageDigest,
						ID:     "mockImageArn",
					},
				},
				LastStatus:    "UNKNOWN",
				StartedAt:     startTime,
				StoppedAt:     stopTime,
				StoppedReason: "some reason",
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			task := Task{
				HealthStatus:  tc.health,
				TaskArn:       tc.taskArn,
				Containers:    tc.containers,
				LastStatus:    tc.lastStatus,
				StartedAt:     &tc.startedAt,
				StoppedAt:     &tc.stoppedAt,
				StoppedReason: tc.stoppedReason,
			}

			gotTaskStatus, gotErr := task.TaskStatus()

			if gotErr != nil {
				require.EqualError(t, tc.wantErr, gotErr.Error())
			} else {
				require.Equal(t, tc.wantTaskStatus, gotTaskStatus)
			}
		})

	}
}

func TestTask_ENI(t *testing.T) {
	testCases := map[string]struct {
		taskARN     *string
		attachments []types.Attachment
		wantedENI   string
		wantedErr   error
	}{
		"no matching attachment": {
			taskARN: awsv2.String("1"),
			attachments: []types.Attachment{
				{
					Type: awsv2.String("not ElasticNetworkInterface"),
				},
			},
			wantedErr: &ErrTaskENIInfoNotFound{
				MissingField: missingFieldAttachment,
				TaskARN:      "1",
			},
		},
		"no matching detail in network interface attachment": {
			taskARN: awsv2.String("1"),
			attachments: []types.Attachment{
				{
					Type: awsv2.String("not ElasticNetworkInterface"),
				},
				{
					Type: awsv2.String("ElasticNetworkInterface"),
					Details: []types.KeyValuePair{
						{
							Name:  awsv2.String("not networkInterfaceId"),
							Value: awsv2.String("val"),
						},
					},
				},
			},
			wantedErr: &ErrTaskENIInfoNotFound{
				MissingField: missingFieldDetailENIID,
				TaskARN:      "1",
			},
		},
		"successfully retrieve eni id": {
			taskARN: awsv2.String("1"),
			attachments: []types.Attachment{
				{
					Type: awsv2.String("not ElasticNetworkInterface"),
				},
				{
					Type: awsv2.String("ElasticNetworkInterface"),
					Details: []types.KeyValuePair{
						{
							Name:  awsv2.String("not networkInterfaceId"),
							Value: awsv2.String("val"),
						},
						{
							Name:  awsv2.String("networkInterfaceId"),
							Value: awsv2.String("eni-123"),
						},
					},
				},
			},
			wantedENI: "eni-123",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			task := Task{
				TaskArn:     tc.taskARN,
				Attachments: tc.attachments,
			}

			out, err := task.ENI()
			if tc.wantedErr != nil {
				require.Equal(t, tc.wantedErr, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.wantedENI, out)
			}
		})
	}
}

func TestTask_PrivateIP(t *testing.T) {
	testCases := map[string]struct {
		taskARN     *string
		attachments []types.Attachment
		wantedENI   string
		wantedErr   error
	}{
		"no matching attachment": {
			taskARN: awsv2.String("1"),
			attachments: []types.Attachment{
				{
					Type: awsv2.String("not ElasticNetworkInterface"),
				},
			},
			wantedErr: &ErrTaskENIInfoNotFound{
				MissingField: missingFieldAttachment,
				TaskARN:      "1",
			},
		},
		"no matching detail in network interface attachment": {
			taskARN: awsv2.String("1"),
			attachments: []types.Attachment{
				{
					Type: awsv2.String("not ElasticNetworkInterface"),
				},
				{
					Type: awsv2.String("ElasticNetworkInterface"),
					Details: []types.KeyValuePair{
						{
							Name:  awsv2.String("not privateIPv4Address"),
							Value: awsv2.String("val"),
						},
					},
				},
			},
			wantedErr: &ErrTaskENIInfoNotFound{
				MissingField: missingFieldPrivateIPv4Address,
				TaskARN:      "1",
			},
		},
		"successfully retrieve eni id": {
			taskARN: awsv2.String("1"),
			attachments: []types.Attachment{
				{
					Type: awsv2.String("not ElasticNetworkInterface"),
				},
				{
					Type: awsv2.String("ElasticNetworkInterface"),
					Details: []types.KeyValuePair{
						{
							Name:  awsv2.String("not networkInterfaceId"),
							Value: awsv2.String("val"),
						},
						{
							Name:  awsv2.String("privateIPv4Address"),
							Value: awsv2.String("eni-123"),
						},
					},
				},
			},
			wantedENI: "eni-123",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			task := Task{
				TaskArn:     tc.taskARN,
				Attachments: tc.attachments,
			}

			out, err := task.PrivateIP()
			if tc.wantedErr != nil {
				require.Equal(t, tc.wantedErr, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.wantedENI, out)
			}
		})
	}
}

func Test_TaskID(t *testing.T) {
	testCases := map[string]struct {
		taskARN string

		wantErr error
		wantID  string
	}{
		"bad unparsable task ARN": {
			taskARN: "mockBadTaskARN",
			wantErr: fmt.Errorf("parse ECS task ARN: arn: invalid prefix"),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// WHEN
			gotID, gotErr := TaskID(tc.taskARN)

			// THEN
			if gotErr != nil {
				require.EqualError(t, gotErr, tc.wantErr.Error())
			} else {
				require.NoError(t, gotErr)
				require.Equal(t, tc.wantID, gotID)
			}
		})

	}
}

func TestTaskDefinition_EnvVars(t *testing.T) {
	testCases := map[string]struct {
		inContainers []types.ContainerDefinition

		wantEnvVars []*ContainerEnvVar
	}{
		"should return wrapped error given error; otherwise should return list of ContainerEnvVar objects": {
			inContainers: []types.ContainerDefinition{
				{
					Environment: []types.KeyValuePair{
						{
							Name:  awsv2.String("COPILOT_SERVICE_NAME"),
							Value: awsv2.String("my-svc"),
						},
						{
							Name:  awsv2.String("COPILOT_ENVIRONMENT_NAME"),
							Value: awsv2.String("prod"),
						},
					},
					Name: awsv2.String("container"),
				},
			},

			wantEnvVars: []*ContainerEnvVar{
				{
					Name:      "COPILOT_SERVICE_NAME",
					Container: "container",
					Value:     "my-svc",
				},
				{
					Name:      "COPILOT_ENVIRONMENT_NAME",
					Container: "container",
					Value:     "prod",
				},
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			taskDefinition := TaskDefinition{
				ContainerDefinitions: tc.inContainers,
			}

			gotEnvVars := taskDefinition.EnvironmentVariables()

			require.Equal(t, tc.wantEnvVars, gotEnvVars)
		})

	}
}

func TestTaskDefinition_Secrets(t *testing.T) {
	testCases := map[string]struct {
		inContainers []types.ContainerDefinition

		wantedSecrets []*ContainerSecret
	}{
		"should return secrets of the task definition as a list of ContainerSecret objects": {
			inContainers: []types.ContainerDefinition{
				{
					Name: awsv2.String("container"),
					Secrets: []types.Secret{
						{
							Name:      awsv2.String("GITHUB_WEBHOOK_SECRET"),
							ValueFrom: awsv2.String("GH_WEBHOOK_SECRET"),
						},
						{
							Name:      awsv2.String("SOME_OTHER_SECRET"),
							ValueFrom: awsv2.String("SHHHHHHHH"),
						},
					},
				},
			},

			wantedSecrets: []*ContainerSecret{
				{
					Name:      "GITHUB_WEBHOOK_SECRET",
					Container: "container",
					ValueFrom: "GH_WEBHOOK_SECRET",
				},
				{
					Name:      "SOME_OTHER_SECRET",
					Container: "container",
					ValueFrom: "SHHHHHHHH",
				},
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			taskDefinition := TaskDefinition{
				ContainerDefinitions: tc.inContainers,
			}

			gotSecrets := taskDefinition.Secrets()

			require.Equal(t, tc.wantedSecrets, gotSecrets)
		})

	}
}

func TestTaskDefinition_Image(t *testing.T) {
	testCases := map[string]struct {
		inContainers    []types.ContainerDefinition
		inContainerName string

		wantedImage string
		wantedError error
	}{
		"should return the container's image": {
			inContainers: []types.ContainerDefinition{
				{
					Name:  awsv2.String("container-1"),
					Image: awsv2.String("image-1"),
				},
				{
					Name:  awsv2.String("container-2"),
					Image: awsv2.String("image-2"),
				},
			},
			inContainerName: "container-2",
			wantedImage:     "image-2",
		},
		"container not found": {
			inContainers: []types.ContainerDefinition{
				{
					Name:  awsv2.String("container-1"),
					Image: awsv2.String("image-1"),
				},
				{
					Name:  awsv2.String("container-2"),
					Image: awsv2.String("image-2"),
				},
			},
			inContainerName: "container-3",
			wantedError:     errors.New("container container-3 not found"),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			taskDefinition := TaskDefinition{
				ContainerDefinitions: tc.inContainers,
			}

			gotImages, err := taskDefinition.Image(tc.inContainerName)
			if tc.wantedError != nil {
				require.EqualError(t, tc.wantedError, err.Error())
			} else {
				require.Equal(t, tc.wantedImage, gotImages)
			}
		})

	}
}

func TestTaskDefinition_Command(t *testing.T) {
	testCases := map[string]struct {
		inContainers    []types.ContainerDefinition
		inContainerName string

		wantedCommand []string
		wantedError   error
	}{
		"should return command overrides of the task definition as a list of ContainerCommand": {
			inContainers: []types.ContainerDefinition{
				{
					Name:    awsv2.String("container-1"),
					Command: []string{"echo", "strikes", "three"},
				},
				{
					Name:    awsv2.String("container-2"),
					Command: []string{"echo", "ball", "four"},
				},
				{
					Name: awsv2.String("container-3"),
				},
			},
			inContainerName: "container-1",
			wantedCommand:   []string{"echo", "strikes", "three"},
		},
		"container not found": {
			inContainers: []types.ContainerDefinition{
				{
					Name:    awsv2.String("container-1"),
					Command: []string{"echo", "strikes", "three"},
				},
				{
					Name:    awsv2.String("container-2"),
					Command: []string{"echo", "ball", "four"},
				},
			},
			inContainerName: "container-3",
			wantedError:     errors.New("container container-3 not found"),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			taskDefinition := TaskDefinition{
				ContainerDefinitions: tc.inContainers,
			}

			gotCommands, err := taskDefinition.Command(tc.inContainerName)
			if tc.wantedError != nil {
				require.EqualError(t, tc.wantedError, err.Error())
			} else {
				require.Equal(t, tc.wantedCommand, gotCommands)
			}
		})

	}
}

func TestTaskDefinition_EntryPoint(t *testing.T) {
	testCases := map[string]struct {
		inContainers    []types.ContainerDefinition
		inContainerName string

		wantedEntryPoints []string
		wantedError       error
	}{
		"should return command overrides of the task definition as a list of ContainerCommand": {
			inContainers: []types.ContainerDefinition{
				{
					Name:       awsv2.String("container-1"),
					EntryPoint: []string{"echo", "strikes", "three"},
				},
				{
					Name: awsv2.String("container-2"),
				},
			},

			inContainerName:   "container-1",
			wantedEntryPoints: []string{"echo", "strikes", "three"},
		},
		"container not found": {
			inContainers: []types.ContainerDefinition{
				{
					Name:    awsv2.String("container-1"),
					Command: []string{"echo", "strikes", "three"},
				},
				{
					Name:    awsv2.String("container-2"),
					Command: []string{"echo", "ball", "four"},
				},
			},
			inContainerName: "container-3",
			wantedError:     errors.New("container container-3 not found"),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			taskDefinition := TaskDefinition{
				ContainerDefinitions: tc.inContainers,
			}

			gotEntryPoints, err := taskDefinition.EntryPoint(tc.inContainerName)
			if tc.wantedError != nil {
				require.EqualError(t, tc.wantedError, err.Error())
			} else {
				require.Equal(t, tc.wantedEntryPoints, gotEntryPoints)
			}
		})

	}
}

func TestShortTaskID(t *testing.T) {
	testCases := map[string]struct {
		inTaskId     string
		wantedTaskId string
	}{
		"return truncated short task id": {
			inTaskId:     "37930fffc2104a1db455aef109b5d122",
			wantedTaskId: "37930fff",
		},
		"return given short taskid": {
			inTaskId:     "37930fff",
			wantedTaskId: "37930fff",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			// WHEN
			got := ShortTaskID(tc.inTaskId)
			// THEN
			require.Equal(t, tc.wantedTaskId, got)
		})

	}
}

func TestFilterRunningTasks(t *testing.T) {
	testCases := map[string]struct {
		inTasks     []*Task
		wantedTasks []*Task
	}{
		"should return only running tasks": {
			inTasks: []*Task{
				{
					TaskArn:    awsv2.String("mockTask1"),
					LastStatus: awsv2.String("STOPPED"),
				},
				{
					TaskArn:    awsv2.String("mockTask2"),
					LastStatus: awsv2.String("RUNNING"),
				},
			},
			wantedTasks: []*Task{
				{
					TaskArn:    awsv2.String("mockTask2"),
					LastStatus: awsv2.String("RUNNING"),
				},
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			got := FilterRunningTasks(tc.inTasks)

			require.Equal(t, tc.wantedTasks, got)
		})

	}
}

func Test_TaskDefinitionVersion(t *testing.T) {
	testCases := map[string]struct {
		inARN string

		wanted      int
		wantedError error
	}{
		"success": {
			inARN:  "arn:aws:ecs:us-east-1:568623488001:task-definition/some-task-def:6",
			wanted: 6,
		},
		"unable to parse": {
			inARN:       "random not ARN",
			wantedError: errors.New("parse ARN random not ARN: arn: invalid prefix"),
		},
		"unable to convert version from string to int": {
			inARN:       "arn:aws:ecs:us-east-1:568623488001:task-definition/some-task-def:six",
			wantedError: errors.New("convert version six from string to int: strconv.Atoi: parsing \"six\": invalid syntax"),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, err := TaskDefinitionVersion(tc.inARN)
			if tc.wantedError != nil {
				require.EqualError(t, err, tc.wantedError.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, got, tc.wanted)
			}
		})
	}
}
