// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package initialize contains methods and structs needed to initialize jobs and services.
package initialize

import (
	"context"
	"encoding"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aproint/copilot-cli/internal/pkg/config"
	"github.com/aproint/copilot-cli/internal/pkg/deploy/cloudformation"
	"github.com/aproint/copilot-cli/internal/pkg/manifest"
	"github.com/aproint/copilot-cli/internal/pkg/manifest/manifestinfo"
	"github.com/aproint/copilot-cli/internal/pkg/metadata"
	"github.com/aproint/copilot-cli/internal/pkg/term/color"
	"github.com/aproint/copilot-cli/internal/pkg/term/log"
	"github.com/aproint/copilot-cli/internal/pkg/workspace"
	"github.com/aws/aws-sdk-go-v2/aws"
)

const (
	jobWlType = "job"
	svcWlType = "service"
)

var fmtErrUnrecognizedWlType = "unrecognized workload type %s"

// Store represents the methods needed to add workloads to the SSM parameter store.
type Store interface {
	GetApplication(ctx context.Context, appName string) (*config.Application, error)
	CreateService(ctx context.Context, service *config.Workload) error
	CreateJob(ctx context.Context, job *config.Workload) error
	ListServices(ctx context.Context, appName string) ([]*config.Workload, error)
	ListJobs(ctx context.Context, appName string) ([]*config.Workload, error)
}

// WorkloadAdder contains the methods needed to add jobs and services to an existing application.
type WorkloadAdder interface {
	AddJobToApp(app *config.Application, jobName string, opts ...cloudformation.AddWorkloadToAppOpt) error
	AddServiceToApp(app *config.Application, serviceName string, opts ...cloudformation.AddWorkloadToAppOpt) error
}

// Workspace contains the methods needed to manipulate a Copilot workspace.
type Workspace interface {
	Rel(path string) (string, error)
	WriteJobManifest(marshaler encoding.BinaryMarshaler, jobName string) (string, error)
	WriteServiceManifest(marshaler encoding.BinaryMarshaler, serviceName string) (string, error)
}

// Prog contains the methods needed to render multi-stage operations.
type Prog interface {
	Start(label string)
	Stop(label string)
}

// WorkloadProps contains the information needed to represent a Workload (job or service).
type WorkloadProps struct {
	App                     string
	Type                    string
	Name                    string
	DockerfilePath          string
	Image                   string
	Platform                manifest.PlatformArgsOrString
	Topics                  []manifest.TopicSubscription
	Queue                   manifest.SQSQueue
	PrivateOnlyEnvironments []string
}

// JobProps contains the information needed to represent a Job.
type JobProps struct {
	WorkloadProps
	Schedule    string
	HealthCheck manifest.ContainerHealthCheck
	Timeout     string
	Retries     int
}

// ServiceProps contains the information needed to represent a Service (port, HealthCheck, and workload common props).
type ServiceProps struct {
	WorkloadProps
	Port        uint16
	HealthCheck manifest.ContainerHealthCheck
	Private     bool
	appDomain   *string
	FileUploads []manifest.FileUpload
}

// WorkloadInitializer holds the clients necessary to initialize either a
// service or job in an existing application.
type WorkloadInitializer struct {
	Store    Store
	Deployer WorkloadAdder
	Ws       Workspace
	Prog     Prog
}

// AddWorkloadToApp contains the logic to create the SSM parameter and perform the stackset template update required
// to add any workload to the app. It does not write the manifest.
func (w *WorkloadInitializer) AddWorkloadToApp(ctx context.Context, appName, name, workloadType string) error {
	svcOrJob := svcWlType
	if manifestinfo.IsTypeAJob(workloadType) {
		svcOrJob = jobWlType
	}

	app, err := w.Store.GetApplication(ctx, appName)
	if err != nil {
		return fmt.Errorf("get application %s: %w", appName, err)
	}
	// addWlToAppandSSM only uses the App, Name, and Type
	return w.addWlToAppAndSSM(ctx, app, WorkloadProps{
		App:  appName,
		Type: workloadType,
		Name: name,
	}, svcOrJob)
}

// Service writes the service manifest, creates an ECR repository, and adds the service to SSM.
func (w *WorkloadInitializer) Service(ctx context.Context, i *ServiceProps) (string, error) {
	return w.initService(ctx, i)
}

// Job writes the job manifest, creates an ECR repository, and adds the job to SSM.
func (w *WorkloadInitializer) Job(ctx context.Context, i *JobProps) (string, error) {
	return w.initJob(ctx, i)
}

func (w *WorkloadInitializer) addWlToApp(app *config.Application, props WorkloadProps, wlType string) error {
	switch wlType {
	case svcWlType:
		if props.Type == manifestinfo.StaticSiteType {
			return w.Deployer.AddServiceToApp(app, props.Name, cloudformation.AddWorkloadToAppOptWithoutECR)
		}
		return w.Deployer.AddServiceToApp(app, props.Name)
	case jobWlType:
		return w.Deployer.AddJobToApp(app, props.Name)
	default:
		return fmt.Errorf(fmtErrUnrecognizedWlType, wlType)
	}
}

func (w *WorkloadInitializer) addWlToStore(ctx context.Context, wl *config.Workload, wlType string) error {
	switch wlType {
	case svcWlType:
		return w.Store.CreateService(ctx, wl)
	case jobWlType:
		return w.Store.CreateJob(ctx, wl)
	default:
		return fmt.Errorf(fmtErrUnrecognizedWlType, wlType)
	}
}

func (w *WorkloadInitializer) initJob(ctx context.Context, props *JobProps) (string, error) {
	if props.DockerfilePath != "" {
		path, err := w.Ws.Rel(props.DockerfilePath)
		if err != nil {
			return "", err
		}
		props.DockerfilePath = path
	}

	var manifestExists bool
	mf, err := newJobManifest(props)
	if err != nil {
		return "", err
	}
	manifestPath, err := w.Ws.WriteJobManifest(mf, props.Name)
	if err != nil {
		e, ok := err.(*workspace.ErrFileExists)
		if !ok {
			return "", fmt.Errorf("write %s manifest: %w", jobWlType, err)
		}
		manifestExists = true
		manifestPath = e.FileName
	}
	manifestMsgFmt := "Wrote the manifest for %s %s at %s\n"
	if manifestExists {
		manifestMsgFmt = "Manifest file for %s %s already exists at %s, skipping writing it.\n"
	}

	path := displayPath(manifestPath)
	log.Successf(manifestMsgFmt, jobWlType, color.HighlightUserInput(props.Name), color.HighlightResource(path))
	var sched = props.Schedule
	if props.Schedule == "" {
		sched = "None"
	}
	helpText := fmt.Sprintf("Your manifest contains configurations like your container size and job schedule (%s).", sched)
	log.Infoln(color.Help(helpText))
	log.Infoln()

	app, err := w.Store.GetApplication(ctx, props.App)
	if err != nil {
		return "", fmt.Errorf("get application %s: %w", props.App, err)
	}

	err = w.addJobToAppAndSSM(ctx, app, props.WorkloadProps)
	if err != nil {
		return "", err
	}

	path, err = w.Ws.Rel(manifestPath)
	if err != nil {
		return "", err
	}
	return path, nil
}

func (w *WorkloadInitializer) initService(ctx context.Context, props *ServiceProps) (string, error) {
	if props.DockerfilePath != "" {
		path, err := w.Ws.Rel(props.DockerfilePath)
		if err != nil {
			return "", err
		}
		props.DockerfilePath = path
	}
	app, err := w.Store.GetApplication(ctx, props.App)
	if err != nil {
		return "", fmt.Errorf("get application %s: %w", props.App, err)
	}
	if app.Domain != "" {
		props.appDomain = aws.String(app.Domain)
	}

	var manifestExists bool
	mf, err := w.newServiceManifest(ctx, props)
	if err != nil {
		return "", err
	}
	manifestPath, err := w.Ws.WriteServiceManifest(mf, props.Name)
	if err != nil {
		e, ok := err.(*workspace.ErrFileExists)
		if !ok {
			return "", fmt.Errorf("write %s manifest: %w", svcWlType, err)
		}
		manifestExists = true
		manifestPath = e.FileName
	}

	manifestMsgFmt := "Wrote the manifest for %s %s at %s\n"
	if manifestExists {
		manifestMsgFmt = "Manifest file for %s %s already exists at %s, skipping writing it.\n"
	}

	path := displayPath(manifestPath)
	log.Successf(manifestMsgFmt, svcWlType, color.HighlightUserInput(props.Name), color.HighlightResource(path))

	helpText := "Your manifest contains configurations like your container size and port."
	log.Infoln(color.Help(helpText))
	log.Infoln()

	err = w.addSvcToAppAndSSM(ctx, app, props.WorkloadProps)
	if err != nil {
		return "", err
	}

	path, err = w.Ws.Rel(manifestPath)
	if err != nil {
		return "", err
	}
	return path, nil
}

func (w *WorkloadInitializer) addSvcToAppAndSSM(ctx context.Context, app *config.Application, props WorkloadProps) error {
	return w.addWlToAppAndSSM(ctx, app, props, svcWlType)
}

func (w *WorkloadInitializer) addJobToAppAndSSM(ctx context.Context, app *config.Application, props WorkloadProps) error {
	return w.addWlToAppAndSSM(ctx, app, props, jobWlType)
}

// addWlToAppAndSSM is a type-agnostic method to add a workload to the app and config store.
func (w *WorkloadInitializer) addWlToAppAndSSM(ctx context.Context, app *config.Application, props WorkloadProps, wlType string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := w.addWlToApp(app, props, wlType); err != nil {
		return fmt.Errorf("add %s %s to application %s: %w", wlType, props.Name, props.App, err)
	}

	if ctx.Err() != nil {
		log.Warningln(metadata.CommitAfterCancellationWarning)
	}
	commitCtx, cancel := metadata.CommitContext(ctx)
	defer cancel()
	if err := w.addWlToStore(commitCtx, &config.Workload{
		App:  props.App,
		Name: props.Name,
		Type: props.Type,
	}, wlType); err != nil {
		return metadata.NewCommitError("workload registration in application stack", fmt.Errorf("saving %s %s: %w", wlType, props.Name, err))
	}

	return nil
}

func newJobManifest(i *JobProps) (encoding.BinaryMarshaler, error) {
	switch i.Type {
	case manifestinfo.ScheduledJobType:
		return manifest.NewScheduledJob(&manifest.ScheduledJobProps{
			WorkloadProps: &manifest.WorkloadProps{
				Name:                    i.Name,
				Dockerfile:              i.DockerfilePath,
				Image:                   i.Image,
				PrivateOnlyEnvironments: i.PrivateOnlyEnvironments,
			},
			HealthCheck: i.HealthCheck,
			Platform:    i.Platform,
			Schedule:    i.Schedule,
			Timeout:     i.Timeout,
			Retries:     i.Retries,
		}), nil
	default:
		return nil, fmt.Errorf("job type %s doesn't have a manifest", i.Type)

	}
}

func (w *WorkloadInitializer) newServiceManifest(ctx context.Context, i *ServiceProps) (encoding.BinaryMarshaler, error) {
	switch i.Type {
	case manifestinfo.LoadBalancedWebServiceType:
		return w.newLoadBalancedWebServiceManifest(ctx, i)
	case manifestinfo.RequestDrivenWebServiceType:
		return newRequestDrivenWebServiceManifest(i), nil
	case manifestinfo.BackendServiceType:
		return w.newBackendServiceManifest(i)
	case manifestinfo.WorkerServiceType:
		return newWorkerServiceManifest(i)
	case manifestinfo.StaticSiteType:
		return newStaticSiteServiceManifest(i)
	default:
		return nil, fmt.Errorf("service type %s doesn't have a manifest", i.Type)
	}
}

func (w *WorkloadInitializer) newLoadBalancedWebServiceManifest(ctx context.Context, inProps *ServiceProps) (*manifest.LoadBalancedWebService, error) {
	outProps := &manifest.LoadBalancedWebServiceProps{
		WorkloadProps: &manifest.WorkloadProps{
			Name:                    inProps.Name,
			Dockerfile:              inProps.DockerfilePath,
			Image:                   inProps.Image,
			PrivateOnlyEnvironments: inProps.PrivateOnlyEnvironments,
		},
		Path:        "/",
		Port:        inProps.Port,
		HealthCheck: inProps.HealthCheck,
		Platform:    inProps.Platform,
	}
	existingSvcs, err := w.Store.ListServices(ctx, inProps.App)
	if err != nil {
		return nil, err
	}
	// We default to "/" for the first service or if the application is initialized with a domain, but if there's another
	// Load Balanced Web Service, we use the svc name as the default, instead.
	if aws.ToString(inProps.appDomain) == "" {
		for _, existingSvc := range existingSvcs {
			if existingSvc.Type == manifestinfo.LoadBalancedWebServiceType && existingSvc.Name != inProps.Name {
				outProps.Path = inProps.Name
				break
			}
		}
	}
	return manifest.NewLoadBalancedWebService(outProps), nil
}

func newRequestDrivenWebServiceManifest(i *ServiceProps) *manifest.RequestDrivenWebService {
	props := &manifest.RequestDrivenWebServiceProps{
		WorkloadProps: &manifest.WorkloadProps{
			Name:       i.Name,
			Dockerfile: i.DockerfilePath,
			Image:      i.Image,
		},
		Port:     i.Port,
		Platform: i.Platform,
		Private:  i.Private,
	}
	return manifest.NewRequestDrivenWebService(props)
}

func (w *WorkloadInitializer) newBackendServiceManifest(i *ServiceProps) (*manifest.BackendService, error) {
	outProps := manifest.BackendServiceProps{
		WorkloadProps: manifest.WorkloadProps{
			Name:                    i.Name,
			Dockerfile:              i.DockerfilePath,
			Image:                   i.Image,
			PrivateOnlyEnvironments: i.PrivateOnlyEnvironments,
		},
		Port:        i.Port,
		HealthCheck: i.HealthCheck,
		Platform:    i.Platform,
	}

	return manifest.NewBackendService(outProps), nil
}

func newWorkerServiceManifest(i *ServiceProps) (*manifest.WorkerService, error) {
	return manifest.NewWorkerService(manifest.WorkerServiceProps{
		WorkloadProps: manifest.WorkloadProps{
			Name:                    i.Name,
			Dockerfile:              i.DockerfilePath,
			Image:                   i.Image,
			PrivateOnlyEnvironments: i.PrivateOnlyEnvironments,
		},
		HealthCheck: i.HealthCheck,
		Platform:    i.Platform,
		Topics:      i.Topics,
		Queue:       i.Queue,
	}), nil
}

func newStaticSiteServiceManifest(i *ServiceProps) (*manifest.StaticSite, error) {
	return manifest.NewStaticSite(manifest.StaticSiteProps{
		Name: i.Name,
		StaticSiteConfig: manifest.StaticSiteConfig{
			FileUploads: i.FileUploads,
		},
	}), nil
}

// Copy of cli.displayPath
func displayPath(target string) string {
	if !filepath.IsAbs(target) {
		return filepath.Clean(target)
	}

	base, err := os.Getwd()
	if err != nil {
		return filepath.Clean(target)
	}

	rel, err := filepath.Rel(base, target)
	if err != nil {
		// No path from base to target available, return target as is.
		return filepath.Clean(target)
	}
	return rel
}
