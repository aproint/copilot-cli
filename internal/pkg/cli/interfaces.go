// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// Copyright APROINT, s.r.o. in modifications to this fork.
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"encoding"
	"io"

	sdkcloudformation "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"

	awscloudformation "github.com/aproint/copilot-cli/internal/pkg/aws/cloudformation"
	"github.com/aproint/copilot-cli/internal/pkg/aws/codepipeline"
	"github.com/aproint/copilot-cli/internal/pkg/aws/ec2"
	awsecs "github.com/aproint/copilot-cli/internal/pkg/aws/ecs"
	"github.com/aproint/copilot-cli/internal/pkg/aws/secretsmanager"
	"github.com/aproint/copilot-cli/internal/pkg/aws/ssm"
	clideploy "github.com/aproint/copilot-cli/internal/pkg/cli/deploy"
	"github.com/aproint/copilot-cli/internal/pkg/config"
	"github.com/aproint/copilot-cli/internal/pkg/deploy"
	"github.com/aproint/copilot-cli/internal/pkg/deploy/cloudformation"
	"github.com/aproint/copilot-cli/internal/pkg/deploy/cloudformation/stack"
	"github.com/aproint/copilot-cli/internal/pkg/describe"
	stackdescr "github.com/aproint/copilot-cli/internal/pkg/describe/stack"
	"github.com/aproint/copilot-cli/internal/pkg/docker/dockerengine"
	"github.com/aproint/copilot-cli/internal/pkg/docker/dockerfile"
	"github.com/aproint/copilot-cli/internal/pkg/ecs"
	"github.com/aproint/copilot-cli/internal/pkg/exec"
	"github.com/aproint/copilot-cli/internal/pkg/initialize"
	"github.com/aproint/copilot-cli/internal/pkg/logging"
	"github.com/aproint/copilot-cli/internal/pkg/manifest"
	"github.com/aproint/copilot-cli/internal/pkg/task"
	"github.com/aproint/copilot-cli/internal/pkg/template"
	"github.com/aproint/copilot-cli/internal/pkg/term/prompt"
	"github.com/aproint/copilot-cli/internal/pkg/term/selector"
	"github.com/aproint/copilot-cli/internal/pkg/workspace"
	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
)

type cmd interface {
	// Validate returns an error if a flag's value is invalid.
	Validate(context.Context) error

	// Ask prompts for flag values that are required but not passed in.
	Ask(context.Context) error

	// Execute runs the command after collecting all required options.
	Execute(context.Context) error
}

// actionCommand is the interface that every command that creates a resource implements.
type actionCommand interface {
	cmd
	// RecommendActions logs a list of follow-up suggestions users can run once the command executes successfully.
	RecommendActions() error
}

// SSM store interfaces.

type serviceStore interface {
	CreateService(ctx context.Context, svc *config.Workload) error
	GetService(ctx context.Context, appName, svcName string) (*config.Workload, error)
	ListServices(ctx context.Context, appName string) ([]*config.Workload, error)
	DeleteService(ctx context.Context, appName, svcName string) error
}

type jobStore interface {
	CreateJob(ctx context.Context, job *config.Workload) error
	GetJob(ctx context.Context, appName, jobName string) (*config.Workload, error)
	ListJobs(ctx context.Context, appName string) ([]*config.Workload, error)
	DeleteJob(ctx context.Context, appName, jobName string) error
}

type wlStore interface {
	ListWorkloads(ctx context.Context, appName string) ([]*config.Workload, error)
	GetWorkload(ctx context.Context, appName, name string) (*config.Workload, error)
}

type workloadListWriter interface {
	Write(ctx context.Context, appName string) error
}

type applicationStore interface {
	applicationCreator
	applicationUpdater
	applicationGetter
	applicationLister
	applicationDeleter
}

type applicationCreator interface {
	CreateApplication(ctx context.Context, app *config.Application) error
}

type applicationUpdater interface {
	UpdateApplication(ctx context.Context, app *config.Application) error
}

type applicationGetter interface {
	GetApplication(ctx context.Context, appName string) (*config.Application, error)
}

type applicationLister interface {
	ListApplications(ctx context.Context) ([]*config.Application, error)
}

type applicationDeleter interface {
	DeleteApplication(ctx context.Context, name string) error
}

type environmentStore interface {
	environmentCreator
	environmentGetter
	environmentLister
	environmentDeleter
	applicationGetter
}

type environmentCreator interface {
	CreateEnvironment(ctx context.Context, env *config.Environment) error
}

type environmentGetter interface {
	GetEnvironment(ctx context.Context, appName string, environmentName string) (*config.Environment, error)
}

type environmentLister interface {
	ListEnvironments(ctx context.Context, appName string) ([]*config.Environment, error)
}

type wsEnvironmentsLister interface {
	ListEnvironments() ([]string, error)
}

type environmentDeleter interface {
	DeleteEnvironment(ctx context.Context, appName, environmentName string) error
}

type store interface {
	applicationStore
	environmentStore
	serviceStore
	jobStore
	wlStore
}

type deployedEnvironmentLister interface {
	ListEnvironmentsDeployedTo(ctx context.Context, appName, svcName string) ([]string, error)
	ListDeployedServices(ctx context.Context, appName, envName string) ([]string, error)
	ListDeployedJobs(ctx context.Context, appName string, envName string) ([]string, error)
	IsServiceDeployed(ctx context.Context, appName, envName string, svcName string) (bool, error)
	ListSNSTopics(ctx context.Context, appName string, envName string) ([]deploy.Topic, error)
}

// Secretsmanager interface.

type secretsManager interface {
	secretCreator
	secretDeleter
}

type secretCreator interface {
	CreateSecret(context.Context, string, string) (string, error)
}

type secretDeleter interface {
	DescribeSecret(context.Context, string) (*secretsmanager.DescribeSecretOutput, error)
	DeleteSecret(context.Context, string) error
}

type imageBuilderPusher interface {
	BuildAndPush(ctx context.Context, args *dockerengine.BuildArguments, w io.Writer) (string, error)
	Build(ctx context.Context, args *dockerengine.BuildArguments, w io.Writer) (string, error)
}

type repositoryLogin interface {
	Login(context.Context) (string, error)
}

type repositoryService interface {
	repositoryLogin
	imageBuilderPusher
}

type ecsClient interface {
	TaskDefinition(context.Context, string, string, string) (*awsecs.TaskDefinition, error)
	ServiceConnectServices(ctx context.Context, app, env, svc string) ([]*awsecs.Service, error)
	DescribeService(context.Context, string, string, string) (*ecs.ServiceDesc, error)
}

type logEventsWriter interface {
	WriteLogEvents(ctx context.Context, opts logging.WriteLogEventsOpts) error
}

type execRunner interface {
	Run(context.Context, string, []string, ...exec.CmdOption) error
}

type eventsWriter interface {
	WriteEventsUntilStopped(context.Context) error
}

type defaultSessionProvider interface {
	DefaultConfig(ctx context.Context) (awsv2.Config, error)
}

type sessionProvider interface {
	defaultSessionProvider
	DefaultConfigWithRegion(ctx context.Context, region string) (awsv2.Config, error)
	ConfigFromRole(ctx context.Context, roleARN string, region string) (awsv2.Config, error)
	ConfigFromProfile(ctx context.Context, name string) (awsv2.Config, error)
	ConfigFromStaticCreds(accessKeyID, secretAccessKey, sessionToken string) (awsv2.Config, error)
}

type describer interface {
	Describe() (describe.HumanJSONStringer, error)
}

type workloadDescriber interface {
	describer
	Manifest(string) ([]byte, error)
}

type wsFileDeleter interface {
	DeleteWorkspaceFile() error
}

type manifestReader interface {
	ReadWorkloadManifest(name string) (workspace.WorkloadManifest, error)
}

type environmentManifestWriter interface {
	WriteEnvironmentManifest(encoding.BinaryMarshaler, string) (string, error)
}

type workspacePathGetter interface {
	Path() string
}

type wsPipelineManifestReader interface {
	ReadPipelineManifest(path string) (*manifest.Pipeline, error)
}

type relPath interface {
	// Rel returns the path relative from the object's root path to the target path.
	//
	// Unlike filepath.Rel, the input path is allowed to be either relative to the
	// current working directory or absolute.
	Rel(path string) (string, error)
}

type wsPipelineIniter interface {
	relPath
	WritePipelineBuildspec(marshaler encoding.BinaryMarshaler, name string) (string, error)
	WritePipelineManifest(marshaler encoding.BinaryMarshaler, name string) (string, error)
	ListPipelines() ([]workspace.PipelineManifest, error)
}

type serviceLister interface {
	ListServices() ([]string, error)
}

type wsSvcReader interface {
	serviceLister
	manifestReader
}

type jobLister interface {
	ListJobs() ([]string, error)
}

type wsJobReader interface {
	manifestReader
	jobLister
}

type wlLister interface {
	ListWorkloads() ([]string, error)
}

type wsWorkloadReader interface {
	manifestReader
	ReadFile(path string) ([]byte, error)
	WorkloadExists(name string) (bool, error)
	WorkloadAddonFilePath(wkldName, fName string) string
	WorkloadAddonFileAbsPath(wkldName, fName string) string
}

type wsWorkloadReadWriter interface {
	wsWorkloadReader
	wsWriter
}

type wsReadWriter interface {
	wsWorkloadReadWriter
	wsEnvironmentReader
}

type wsJobDirReader interface {
	wsJobReader
	workspacePathGetter
}

type wsWlDirReader interface {
	wsJobReader
	wsSvcReader
	workspacePathGetter
	wlLister
	wsEnvironmentsLister
	WorkloadOverridesPath(string) string
	Summary() (*workspace.Summary, error)
}

type wsEnvironmentReader interface {
	wsEnvironmentsLister
	HasEnvironments() (bool, error)
	EnvOverridesPath() string
	ReadEnvironmentManifest(mftDirName string) (workspace.EnvironmentManifest, error)
	EnvAddonFilePath(fName string) string
	EnvAddonFileAbsPath(fName string) string
}

type wsPipelineReader interface {
	wsPipelineGetter
	wsPipelineManifestReader
	relPath
	PipelineOverridesPath(string) string
}

type wsPipelineGetter interface {
	wsPipelineManifestReader
	wlLister
	ListPipelines() ([]workspace.PipelineManifest, error)
}

type wsAppManager interface {
	Summary() (*workspace.Summary, error)
}

type wsAppManagerDeleter interface {
	wsAppManager
	wsFileDeleter
}

type wsWriter interface {
	Write(content encoding.BinaryMarshaler, path string) (string, error)
}

type uploader interface {
	Upload(ctx context.Context, bucket, key string, data io.Reader) (string, error)
}

type bucketEmptier interface {
	EmptyBucket(ctx context.Context, bucket string) error
}

type stackDescriber interface {
	Resources(context.Context) ([]*stackdescr.Resource, error)
}

// Interfaces for deploying resources through CloudFormation. Facilitates mocking.
type environmentDeployer interface {
	CreateAndRenderEnvironment(context.Context, cloudformation.StackConfiguration, string) error
	DeleteEnvironment(context.Context, string, string, string) error
	GetEnvironment(ctx context.Context, appName, envName string) (*config.Environment, error)
	Template(context.Context, string) (string, error)
	UpdateEnvironmentTemplate(context.Context, string, string, string, string) error
}

type wlDeleter interface {
	DeleteWorkload(context.Context, deploy.DeleteWorkloadInput) error
}

type svcRemoverFromApp interface {
	RemoveServiceFromApp(context.Context, *config.Application, string) error
}

type jobRemoverFromApp interface {
	RemoveJobFromApp(context.Context, *config.Application, string) error
}

type imageRemover interface {
	ClearRepository(ctx context.Context, repoName string) error // implemented by ECR Service
}

type pipelineDeployer interface {
	CreatePipeline(context.Context, string, cloudformation.StackConfiguration) error
	UpdatePipeline(context.Context, string, cloudformation.StackConfiguration) error
	PipelineExists(context.Context, cloudformation.StackConfiguration) (bool, error)
	DeletePipeline(context.Context, deploy.Pipeline) error
	AddPipelineResourcesToApp(context.Context, *config.Application, string) error
	Template(context.Context, string) (string, error)
	appResourcesGetter
	// TODO: Add StreamPipelineCreation method
}

type appDeployer interface {
	DeployApp(context.Context, *deploy.CreateAppInput) error
	AddServiceToApp(context.Context, *config.Application, string, ...cloudformation.AddWorkloadToAppOpt) error
	AddJobToApp(context.Context, *config.Application, string, ...cloudformation.AddWorkloadToAppOpt) error
	AddEnvToApp(context.Context, *cloudformation.AddEnvToAppOpts) error
	DelegateDNSPermissions(context.Context, *config.Application, string) error
	DeleteApp(context.Context, string) error
}

type appResourcesGetter interface {
	GetAppResourcesByRegion(context.Context, *config.Application, string) (*stack.AppRegionalResources, error)
	GetRegionalAppResources(context.Context, *config.Application) ([]*stack.AppRegionalResources, error)
}

type envDeleterFromApp interface {
	appResourcesGetter
	RemoveEnvFromApp(context.Context, *cloudformation.RemoveEnvFromAppOpts) error
}

type taskDeployer interface {
	DeployTask(ctx context.Context, input *deploy.CreateTaskResourcesInput, opts ...awscloudformation.StackOption) error
	GetTaskStack(ctx context.Context, taskName string) (*deploy.TaskStackInfo, error)
}

type taskStackManager interface {
	DeleteTask(ctx context.Context, task deploy.TaskStackInfo) error
	GetTaskStack(ctx context.Context, taskName string) (*deploy.TaskStackInfo, error)
}

type taskRunner interface {
	Run(context.Context) ([]*task.Task, error)
	CheckNonZeroExitCode(context.Context, []*task.Task) error
}

type defaultClusterGetter interface {
	HasDefaultCluster(context.Context) (bool, error)
}

type deployer interface {
	environmentDeployer
	appDeployer
	pipelineDeployer
	ListTaskStacks(context.Context, string, string) ([]deploy.TaskStackInfo, error)
}

type domainHostedZoneGetter interface {
	PublicDomainHostedZoneID(context.Context, string) (string, error)
	ValidateDomainOwnership(context.Context, string) error
}

type dockerfileParser interface {
	GetExposedPorts() ([]dockerfile.Port, error)
	GetHealthCheck() (*dockerfile.HealthCheck, error)
}

type statusDescriber interface {
	Describe() (describe.HumanJSONStringer, error)
}

type envDescriber interface {
	Describe() (*describe.EnvDescription, error)
	Manifest() ([]byte, error)
	ValidateCFServiceDomainAliases() error
}

type versionCompatibilityChecker interface {
	versionGetter
	AvailableFeatures() ([]string, error)
}

type versionGetter interface {
	Version() (string, error)
}

type appUpgrader interface {
	UpgradeApplication(context.Context, *deploy.CreateAppInput) error
}

type pipelineGetter interface {
	GetPipeline(ctx context.Context, pipelineName string) (*codepipeline.Pipeline, error)
}

type deployedPipelineLister interface {
	ListDeployedPipelines(ctx context.Context, appName string) ([]deploy.Pipeline, error)
}

type executor interface {
	Execute(context.Context) error
}

type executeAsker interface {
	Ask(context.Context) error
	executor
}

type appSelector interface {
	Application(ctx context.Context, prompt, help string, additionalOpts ...string) (string, error)
}

type appEnvSelector interface {
	appSelector
	Environment(ctx context.Context, prompt, help, app string, additionalOpts ...prompt.Option) (string, error)
}

type cfnSelector interface {
	Resources(msg, finalMsg, help, body string) ([]template.CFNResource, error)
}

type configSelector interface {
	appEnvSelector
	Service(ctx context.Context, prompt, help, app string) (string, error)
	Job(ctx context.Context, prompt, help, app string) (string, error)
	Workload(ctx context.Context, prompt, help, app string) (string, error)
}

type deploySelector interface {
	appSelector
	DeployedService(ctx context.Context, prompt, help string, app string, opts ...selector.GetDeployedWorkloadOpts) (*selector.DeployedService, error)
	DeployedJob(ctx context.Context, prompt, help string, app string, opts ...selector.GetDeployedWorkloadOpts) (*selector.DeployedJob, error)
	DeployedWorkload(ctx context.Context, prompt, help string, app string, opts ...selector.GetDeployedWorkloadOpts) (*selector.DeployedWorkload, error)
}

type pipelineEnvSelector interface {
	Environments(ctx context.Context, prompt, help, app string, finalMsgFunc func(int) prompt.PromptConfig) ([]string, error)
}

type wsPipelineSelector interface {
	WsPipeline(prompt, help string) (*workspace.PipelineManifest, error)
}

type wsEnvironmentSelector interface {
	LocalEnvironment(ctx context.Context, msg, help string) (wl string, err error)
}

type codePipelineSelector interface {
	appSelector
	DeployedPipeline(ctx context.Context, prompt, help, app string) (deploy.Pipeline, error)
}

type wsSelector interface {
	appEnvSelector
	Service(ctx context.Context, prompt, help string) (string, error)
	Job(ctx context.Context, prompt, help string) (string, error)
	Workload(ctx context.Context, msg, help string) (string, error)
	Workloads(ctx context.Context, msg, help string) ([]string, error)
}

type staticSourceSelector interface {
	StaticSources(selPrompt, selHelp, anotherPathPrompt, anotherPathHelp string, pathValidator prompt.ValidatorFunc) ([]string, error)
}

type scheduleSelector interface {
	Schedule(scheduleTypePrompt, scheduleTypeHelp string, scheduleValidator, rateValidator prompt.ValidatorFunc) (string, error)
}

type cfTaskSelector interface {
	Task(ctx context.Context, prompt, help string, opts ...selector.GetDeployedTaskOpts) (string, error)
}

type dockerfileSelector interface {
	Dockerfile(selPrompt, notFoundPrompt, selHelp, notFoundHelp string, pv prompt.ValidatorFunc) (string, error)
}

type topicSelector interface {
	Topics(ctx context.Context, prompt, help, app string) ([]deploy.Topic, error)
}

type ec2Selector interface {
	VPC(ctx context.Context, prompt, help string) (string, error)
	Subnets(ctx context.Context, input selector.SubnetsInput) ([]string, error)
}

type credsSelector interface {
	Creds(ctx context.Context, prompt, help string) (awsv2.Config, error)
}

type ec2Client interface {
	HasDNSSupport(ctx context.Context, vpcID string) (bool, error)
	ListAZs(ctx context.Context) ([]ec2.AZ, error)
}

type serviceResumer interface {
	ResumeService(context.Context, string) error
}

type jobInitializer interface {
	Job(ctx context.Context, props *initialize.JobProps) (string, error)
}

type svcInitializer interface {
	Service(ctx context.Context, props *initialize.ServiceProps) (string, error)
}

type wkldInitializerWithoutManifest interface {
	AddWorkloadToApp(ctx context.Context, appName, name, workloadType string) error
}

type roleDeleter interface {
	DeleteRole(context.Context, string) error
}

type policyLister interface {
	ListPolicyNames(context.Context) ([]string, error)
}

type serviceDescriber interface {
	DescribeService(ctx context.Context, app, env, svc string) (*ecs.ServiceDesc, error)
}

type apprunnerServiceDescriber interface {
	ServiceARN(env string) (string, error)
}

type ecsCommandExecutor interface {
	ExecuteCommand(ctx context.Context, in awsecs.ExecuteCommandInput) error
}

type ssmPluginManager interface {
	ValidateBinary(context.Context) error
	InstallLatestBinary(context.Context) error
}

type taskStopper interface {
	StopOneOffTasks(ctx context.Context, app, env, family string) error
	StopDefaultClusterTasks(ctx context.Context, familyName string) error
	StopWorkloadTasks(context.Context, string, string, string) error
}

type serviceLinkedRoleCreator interface {
	CreateECSServiceLinkedRole(context.Context) error
}

type roleTagsLister interface {
	ListRoleTags(context.Context, string) (map[string]string, error)
}

type roleManager interface {
	roleTagsLister
	roleDeleter
	serviceLinkedRoleCreator
}

type stackExistChecker interface {
	Exists(context.Context, string) (bool, error)
}

type runningTaskSelector interface {
	RunningTask(ctx context.Context, prompt, help string, opts ...selector.TaskOpts) (*awsecs.Task, error)
}

type dockerEngine interface {
	CheckDockerEngineRunning(context.Context) error
	GetPlatform(context.Context) (string, string, error)
}

type codestar interface {
	GetConnectionARN(context.Context, string) (string, error)
}

type publicIPGetter interface {
	PublicIP(ctx context.Context, ENI string) (string, error)
}

type cliStringer interface {
	CLIString() (string, error)
}

type secretPutter interface {
	PutSecret(context.Context, ssm.PutSecretInput) (*ssm.PutSecretOutput, error)
}

type servicePauser interface {
	PauseService(ctx context.Context, svcARN string) error
}

type interpolator interface {
	Interpolate(s string) (string, error)
}

type workloadDeployer interface {
	UploadArtifacts(context.Context) (*clideploy.UploadArtifactsOutput, error)
	GenerateCloudFormationTemplate(context.Context, *clideploy.GenerateCloudFormationTemplateInput) (
		*clideploy.GenerateCloudFormationTemplateOutput, error)
	DeployWorkload(context.Context, *clideploy.DeployWorkloadInput) (clideploy.ActionRecommender, error)
	IsServiceAvailableInRegion(region string) (bool, error)
	templateDiffer
}

type templateDiffer interface {
	DeployDiff(inTmpl string) (string, error)
}

type dockerEngineRunner interface {
	CheckDockerEngineRunning(context.Context) error
	Run(context.Context, *dockerengine.RunOptions) error
	IsContainerRunning(context.Context, string) (bool, error)
	Stop(context.Context, string) error
	Build(context.Context, *dockerengine.BuildArguments, io.Writer) error
	Exec(ctx context.Context, container string, out io.Writer, cmd string, args ...string) error
	ContainerExitCode(ctx context.Context, containerName string) (int, error)
	IsContainerHealthy(ctx context.Context, containerName string) (bool, error)
	Rm(context.Context, string) error
}

type workloadStackGenerator interface {
	UploadArtifacts(context.Context) (*clideploy.UploadArtifactsOutput, error)
	GenerateCloudFormationTemplate(context.Context, *clideploy.GenerateCloudFormationTemplateInput) (
		*clideploy.GenerateCloudFormationTemplateOutput, error)
	AddonsTemplate() (string, error)
	templateDiffer
}

type runner interface {
	Run(context.Context) error
}

type envDeployer interface {
	DeployEnvironment(context.Context, *clideploy.DeployEnvironmentInput) error
	Validate(*manifest.Environment) error
	UploadArtifacts(context.Context) (*clideploy.UploadEnvArtifactsOutput, error)
	GenerateCloudFormationTemplate(context.Context, *clideploy.DeployEnvironmentInput) (
		*clideploy.GenerateCloudFormationTemplateOutput, error)
	templateDiffer
}

type envPackager interface {
	GenerateCloudFormationTemplate(context.Context, *clideploy.DeployEnvironmentInput) (*clideploy.GenerateCloudFormationTemplateOutput, error)
	Validate(*manifest.Environment) error
	UploadArtifacts(context.Context) (*clideploy.UploadEnvArtifactsOutput, error)
	AddonsTemplate() (string, error)
	templateDiffer
}

type stackConfiguration interface {
	StackName() string
	Template() (string, error)
	Parameters() ([]*sdkcloudformation.Parameter, error)
	Tags() []*sdkcloudformation.Tag
	SerializedParameters() (string, error)
}

type secretGetter interface {
	GetSecretValue(context.Context, string) (string, error)
}

type dockerWorkload interface {
	Dockerfile() string
}
