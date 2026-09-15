// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package cloudformation provides functionality to deploy CLI concepts with AWS CloudFormation.
package cloudformation

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aproint/copilot-cli/internal/pkg/aws/ecr"
	"github.com/aproint/copilot-cli/internal/pkg/deploy/cloudformation/stack"

	"github.com/aproint/copilot-cli/internal/pkg/aws/cloudformation"
	"github.com/aproint/copilot-cli/internal/pkg/aws/cloudformation/stackset"
	"github.com/aproint/copilot-cli/internal/pkg/aws/cloudwatch"
	"github.com/aproint/copilot-cli/internal/pkg/aws/codepipeline"
	"github.com/aproint/copilot-cli/internal/pkg/aws/codestar"
	"github.com/aproint/copilot-cli/internal/pkg/aws/ecs"
	"github.com/aproint/copilot-cli/internal/pkg/aws/s3"
	"github.com/aproint/copilot-cli/internal/pkg/deploy"
	"github.com/aproint/copilot-cli/internal/pkg/interrupt"
	"github.com/aproint/copilot-cli/internal/pkg/stream"
	"github.com/aproint/copilot-cli/internal/pkg/term/color"
	"github.com/aproint/copilot-cli/internal/pkg/term/cursor"
	"github.com/aproint/copilot-cli/internal/pkg/term/log"
	"github.com/aproint/copilot-cli/internal/pkg/term/progress"
	"github.com/aws/aws-sdk-go-v2/aws"
	sdkcloudformation "github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"golang.org/x/sync/errgroup"
)

const (
	// waitForStackTimeout is how long we're willing to wait for a stack to go from in progress to a complete state.
	waitForStackTimeout = 1*time.Hour + 30*time.Minute

	// CloudFormation resource types.
	ecsServiceResourceType    = "AWS::ECS::Service"
	envControllerResourceType = "Custom::EnvControllerFunction"
)

// CloudFormation's error types to compare against.
var (
	errNotFound *cloudformation.ErrStackNotFound
)

// StackConfiguration represents the set of methods needed to deploy a cloudformation stack.
type StackConfiguration interface {
	StackName() string
	Template() (string, error)
	Parameters() ([]*types.Parameter, error)
	Tags() []*types.Tag
	SerializedParameters() (string, error)
}

// An Overrider transforms the content in body to out.
type Overrider interface {
	Override(body []byte) (out []byte, err error)
}

// overridableStack is a StackConfiguration with overrides applied.
type overridableStack struct {
	StackConfiguration
	overrider Overrider
}

// Template returns the overriden CloudFormation stack template.
func (s *overridableStack) Template() (string, error) {
	tpl, err := s.StackConfiguration.Template()
	if err != nil {
		return "", fmt.Errorf("generate stack template: %w", err)
	}
	out, err := s.overrider.Override([]byte(tpl))
	if err != nil {
		return "", fmt.Errorf("override template: %w", err)
	}
	return string(out), nil
}

// WrapWithTemplateOverrider returns a wrapped stack, such that Template calls returns an overriden stack template.
func WrapWithTemplateOverrider(stack StackConfiguration, overrider Overrider) StackConfiguration {
	return &overridableStack{
		StackConfiguration: stack,
		overrider:          overrider,
	}
}

type ecsClient interface {
	stream.ECSServiceDescriber
}

type cwClient interface {
	stream.CloudWatchDescriber
}

type cfnClient interface {
	// Methods augmented by the aws wrapper struct.
	Create(*cloudformation.Stack) (string, error)
	CreateWithContext(context.Context, *cloudformation.Stack) (string, error)
	CreateAndWait(*cloudformation.Stack) error
	WaitForCreate(ctx context.Context, stackName string) error
	Update(*cloudformation.Stack) (string, error)
	UpdateWithContext(context.Context, *cloudformation.Stack) (string, error)
	UpdateAndWait(*cloudformation.Stack) error
	WaitForUpdate(ctx context.Context, stackName string) error
	Delete(stackName string) error
	DeleteAndWait(stackName string) error
	DeleteAndWaitWithContext(context.Context, string) error
	DeleteAndWaitWithRoleARN(stackName, roleARN string) error
	Describe(stackName string) (*cloudformation.StackDescription, error)
	DescribeWithContext(ctx context.Context, stackName string) (*cloudformation.StackDescription, error)
	DescribeChangeSet(changeSetID, stackName string) (*cloudformation.ChangeSetDescription, error)
	DescribeChangeSetWithContext(context.Context, string, string) (*cloudformation.ChangeSetDescription, error)
	TemplateBody(stackName string) (string, error)
	TemplateBodyWithContext(context.Context, string) (string, error)
	TemplateBodyFromChangeSet(changeSetID, stackName string) (string, error)
	TemplateBodyFromChangeSetWithContext(context.Context, string, string) (string, error)
	Events(stackName string) ([]cloudformation.StackEvent, error)
	ListStacksWithTags(tags map[string]string) ([]cloudformation.StackDescription, error)
	ErrorEvents(stackName string) ([]cloudformation.StackEvent, error)
	ErrorEventsWithContext(context.Context, string) ([]cloudformation.StackEvent, error)
	Outputs(stack *cloudformation.Stack) (map[string]string, error)
	StackResources(name string) ([]*cloudformation.StackResource, error)
	Metadata(opts cloudformation.MetadataOpts) (string, error)
	MetadataWithContext(context.Context, cloudformation.MetadataOpts) (string, error)
	CancelUpdateStack(stackName string) error
	CancelUpdateStackWithContext(context.Context, string) error

	// Methods vended by the aws sdk struct.
	DescribeStackEvents(*sdkcloudformation.DescribeStackEventsInput) (*sdkcloudformation.DescribeStackEventsOutput, error)
	DescribeStackEventsWithContext(context.Context, *sdkcloudformation.DescribeStackEventsInput) (*sdkcloudformation.DescribeStackEventsOutput, error)
}

type codeStarClient interface {
	WaitUntilConnectionStatusAvailable(ctx context.Context, connectionARN string) error
}

type codePipelineClient interface {
	RetryStageExecution(pipelineName, stageName string) error
}

type s3Client interface {
	Upload(bucket, fileName string, data io.Reader) (string, error)
	UploadWithContext(ctx context.Context, bucket, fileName string, data io.Reader) (string, error)
	EmptyBucket(bucket string) error
}

type imageRemover interface {
	ClearRepository(repoName string) error
}

type stackSetClient interface {
	Create(name, template string, opts ...stackset.CreateOrUpdateOption) error
	CreateWithContext(context.Context, string, string, ...stackset.CreateOrUpdateOption) error
	CreateInstances(name string, accounts, regions []string) (string, error)
	CreateInstancesAndWait(name string, accounts, regions []string) error
	Update(name, template string, opts ...stackset.CreateOrUpdateOption) (string, error)
	UpdateWithContext(context.Context, string, string, ...stackset.CreateOrUpdateOption) (string, error)
	UpdateAndWait(name, template string, opts ...stackset.CreateOrUpdateOption) error
	Describe(name string) (stackset.Description, error)
	DescribeWithContext(context.Context, string) (stackset.Description, error)
	DescribeOperation(name, opID string) (stackset.Operation, error)
	DescribeOperationWithContext(context.Context, string, string) (stackset.Operation, error)
	InstanceSummaries(name string, opts ...stackset.InstanceSummariesOption) ([]stackset.InstanceSummary, error)
	InstanceSummariesWithContext(context.Context, string, ...stackset.InstanceSummariesOption) ([]stackset.InstanceSummary, error)
	DeleteInstance(name, account, region string) (string, error)
	DeleteAllInstances(name string) (string, error)
	Delete(name string) error
	WaitForStackSetLastOperationComplete(name string) error
	WaitForStackSetLastOperationCompleteWithContext(context.Context, string) error
	WaitForOperation(name, opID string) error
	WaitForOperationWithContext(context.Context, string, string) error
}

// OptFn represents an optional configuration function for the CloudFormation client.
type OptFn func(cfn *CloudFormation)

// WithProgressTracker updates the CloudFormation client to write stack updates to a file.
func WithProgressTracker(fw progress.FileWriter) OptFn {
	return func(cfn *CloudFormation) {
		cfn.console = fw
	}
}

// discardFile represents a fake file where all Writes succeeds and are not written anywhere.
type discardFile struct{}

// Write implements the io.Writer interface and discards p.
func (f *discardFile) Write(p []byte) (n int, err error) { return io.Discard.Write(p) }

// Fd returns stderr as the file descriptor.
// The file descriptor value shouldn't matter as long as it's a valid value as all writes are gone to io.Discard.
func (f *discardFile) Fd() uintptr {
	return os.Stderr.Fd()
}

// CloudFormation wraps the CloudFormationAPI interface
type CloudFormation struct {
	cfnClient         cfnClient
	codeStarClient    codeStarClient
	cpClient          codePipelineClient
	ecsClient         ecsClient
	cwClient          cwClient
	regionalClient    func(region string) cfnClient
	appStackSet       stackSetClient
	s3Client          s3Client
	regionalS3Client  func(region string) s3Client
	regionalECRClient func(region string) imageRemover
	region            string
	console           progress.FileWriter

	// cached variables.
	cachedDeployedStack *cloudformation.StackDescription

	// Overridden in tests.
	renderStackSet               func(context.Context, renderStackSetInput) error
	dnsDelegatedAccountsForStack func(stack *types.Stack) []string
}

// New returns a configured CloudFormation client.
func New(v2Config aws.Config, opts ...OptFn) CloudFormation {
	client := CloudFormation{
		cfnClient:      cloudformation.New(v2Config),
		codeStarClient: codestar.New(v2Config),
		cpClient:       codepipeline.New(v2Config, v2Config),
		ecsClient:      ecs.New(v2Config),
		cwClient:       cloudwatch.New(v2Config, v2Config),
		regionalClient: func(region string) cfnClient {
			regionalV2Config := v2Config
			regionalV2Config.Region = region
			return cloudformation.New(regionalV2Config)
		},
		regionalECRClient: func(region string) imageRemover {
			regionalV2Config := v2Config
			regionalV2Config.Region = region
			return ecr.New(regionalV2Config)
		},
		appStackSet: stackset.New(v2Config),
		s3Client:    s3.New(v2Config),
		regionalS3Client: func(region string) s3Client {
			regionalV2Config := v2Config
			regionalV2Config.Region = region
			return s3.New(regionalV2Config)
		},
		region:  v2Config.Region,
		console: new(discardFile),
	}
	for _, opt := range opts {
		opt(&client)
	}
	client.renderStackSet = client.renderStackSetImpl
	client.dnsDelegatedAccountsForStack = stack.DNSDelegatedAccountsForStack
	return client
}

// Template returns a deployed stack's template.
func (cf CloudFormation) Template(stackName string) (string, error) {
	return cf.cfnClient.TemplateBody(stackName)
}

// IsEmptyErr returns true if the error occurred because the cloudformation resource does not exist or does not contain any sub-resources.
func IsEmptyErr(err error) bool {
	type isEmpty interface {
		IsEmpty() bool
	}

	var emptyErr isEmpty
	return errors.As(err, &emptyErr)
}

// errorEvents returns the list of status reasons of failed resource events
func (cf CloudFormation) errorEvents(ctx context.Context, stackName string) ([]string, error) {
	events, err := cf.cfnClient.ErrorEventsWithContext(ctx, stackName)
	if err != nil {
		return nil, err
	}
	var reasons []string
	for _, event := range events {
		// CFN error messages end with a '. (Service' and only the first sentence is useful, the rest is error codes.
		reasons = append(reasons, strings.Split(aws.ToString(event.ResourceStatusReason), ". (Service")[0])
	}
	return reasons, nil
}

type executeAndRenderChangeSetInput struct {
	stackName        string
	stackDescription string
	createChangeSet  func(context.Context) (string, error)
	enableInterrupt  bool
	detach           bool
}

type executeAndRenderChangeSetOption func(in *executeAndRenderChangeSetInput)

func withEnableInterrupt() executeAndRenderChangeSetOption {
	return func(in *executeAndRenderChangeSetInput) {
		in.enableInterrupt = true
	}
}

func withDetach(detach bool) executeAndRenderChangeSetOption {
	return func(in *executeAndRenderChangeSetInput) {
		in.detach = detach
	}
}

func (cf CloudFormation) newCreateChangeSetInput(w progress.FileWriter, stack *cloudformation.Stack) *executeAndRenderChangeSetInput {
	in := &executeAndRenderChangeSetInput{
		stackName:        stack.Name,
		stackDescription: fmt.Sprintf("Creating the infrastructure for stack %s", stack.Name),
	}
	in.createChangeSet = func(ctx context.Context) (string, error) {
		spinner := progress.NewSpinner(w)
		label := fmt.Sprintf("Proposing infrastructure changes for stack %s", stack.Name)
		spinner.Start(label)

		var errAlreadyExists *cloudformation.ErrStackAlreadyExists
		changeSetID, err := cf.cfnClient.CreateWithContext(ctx, stack)
		if err != nil && !errors.As(err, &errAlreadyExists) {
			spinner.Stop(log.Serrorf("%s\n", label))
			return "", cf.handleStackError(ctx, stack.Name, err)
		}
		spinner.Stop(log.Ssuccessf("%s\n", label))
		return changeSetID, err
	}
	return in
}

func (cf CloudFormation) newUpsertChangeSetInput(w progress.FileWriter, stack *cloudformation.Stack, opts ...executeAndRenderChangeSetOption) *executeAndRenderChangeSetInput {
	in := &executeAndRenderChangeSetInput{
		stackName:        stack.Name,
		stackDescription: fmt.Sprintf("Creating the infrastructure for stack %s", stack.Name),
	}
	in.createChangeSet = func(ctx context.Context) (changeSetID string, err error) {
		spinner := progress.NewSpinner(w)
		label := fmt.Sprintf("Proposing infrastructure changes for stack %s", stack.Name)
		spinner.Start(label)
		changeSetID, err = cf.cfnClient.CreateWithContext(ctx, stack)
		if err == nil {
			// Successfully created the change set to create the stack.
			spinner.Stop(log.Ssuccessf("%s\n", label))
			return changeSetID, nil
		}

		var errAlreadyExists *cloudformation.ErrStackAlreadyExists
		if !errors.As(err, &errAlreadyExists) {
			// Unexpected error trying to create a stack.
			spinner.Stop(log.Serrorf("%s\n", label))
			return "", cf.handleStackError(ctx, stack.Name, err)
		}

		// We have to create an update stack change set instead.
		in.stackDescription = fmt.Sprintf("Updating the infrastructure for stack %s", stack.Name)
		changeSetID, err = cf.cfnClient.UpdateWithContext(ctx, stack)
		if err != nil {
			msg := log.Serrorf("%s\n", label)
			var errChangeSetEmpty *cloudformation.ErrChangeSetEmpty
			if errors.As(err, &errChangeSetEmpty) {
				msg = fmt.Sprintf("- No new infrastructure changes for stack %s\n", stack.Name)
			}
			spinner.Stop(msg)
			return "", cf.handleStackError(ctx, stack.Name, err)
		}
		spinner.Stop(log.Ssuccessf("%s\n", label))
		return changeSetID, nil
	}
	for _, opt := range opts {
		opt(in)
	}
	return in
}

func (cf CloudFormation) executeAndRenderChangeSet(ctx context.Context, in *executeAndRenderChangeSetInput) error {
	changeSetID, err := in.createChangeSet(ctx)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return err
	}
	if in.detach {
		return nil
	}
	var interruptCh <-chan struct{}
	if in.enableInterrupt {
		interruptCh = interrupt.FromContext(ctx)
	}
	g, groupCtx := errgroup.WithContext(ctx)
	renderCtx, cancel := context.WithCancel(groupCtx)
	defer cancel()
	prevChangeSetRenderComplete := make(chan bool)
	localInterrupt := make(chan struct{})
	g.Go(func() error {
		defer close(prevChangeSetRenderComplete)
		defer cancel()
		nl, err := cf.renderChangeSet(renderCtx, changeSetID, in)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				select {
				case <-localInterrupt:
					cursor.EraseLinesAbove(cf.console, nl)
					return nil
				case <-interruptCh:
					cursor.EraseLinesAbove(cf.console, nl)
					return nil
				default:
				}
			}
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			return err
		}
		return nil
	})
	if in.enableInterrupt {
		g.Go(func() error {
			err := cf.waitForInterruptAndHandle(interruptHandlerInput{
				ctx:              renderCtx,
				cancelFn:         cancel,
				localInterrupt:   localInterrupt,
				interruptCh:      interruptCh,
				stackName:        in.stackName,
				updateRenderDone: prevChangeSetRenderComplete,
			})
			if errors.Is(err, context.Canceled) && ctx.Err() == nil {
				return nil
			}
			return err
		})
	}
	return g.Wait()
}

// renderChangeSet renders and executes a CloudFormation change set, providing progress updates if necessary.
// It returns the number of rendered lines and any encountered error.
func (cf CloudFormation) renderChangeSet(ctx context.Context, changeSetID string, in *executeAndRenderChangeSetInput) (int, error) {
	if _, ok := cf.console.(*discardFile); ok { // If we don't have to render skip the additional network calls.
		return 0, nil
	}
	waitCtx, cancelWait := context.WithTimeout(ctx, waitForStackTimeout)
	defer cancelWait()
	g, renderCtx := errgroup.WithContext(waitCtx)

	renderer, err := cf.createChangeSetRenderer(g, renderCtx, changeSetID, in.stackName, in.stackDescription, progress.RenderOptions{})
	if err != nil {
		return 0, err
	}
	var prevNumLines int
	g.Go(func() error {
		var err error
		prevNumLines, err = progress.Render(renderCtx, progress.NewTabbedFileWriter(cf.console), renderer)
		return err
	})
	if err := g.Wait(); err != nil {
		return prevNumLines, err
	}
	if err := cf.errOnFailedStack(waitCtx, in.stackName); err != nil {
		return prevNumLines, err
	}
	return prevNumLines, nil
}

type interruptHandlerInput struct {
	ctx              context.Context
	cancelFn         context.CancelFunc
	localInterrupt   chan struct{}
	interruptCh      <-chan struct{}
	stackName        string
	updateRenderDone chan bool
}

func (cf CloudFormation) waitForInterruptAndHandle(in interruptHandlerInput) error {
	for {
		select {
		case <-in.interruptCh:
			return cf.handleInterrupt(in)
		default:
		}
		select {
		case <-in.interruptCh:
			return cf.handleInterrupt(in)
		case <-in.ctx.Done():
			select {
			case <-in.interruptCh:
				return cf.handleInterrupt(in)
			default:
			}
			return in.ctx.Err()
		}
	}
}

func (cf CloudFormation) handleInterrupt(in interruptHandlerInput) error {
	close(in.localInterrupt)
	in.cancelFn()
	cleanupCtx, cancel := cleanupContext(in.ctx)
	defer cancel()
	stackDescr, err := cf.cfnClient.DescribeWithContext(cleanupCtx, in.stackName)
	if err != nil {
		return fmt.Errorf("describe stack %s: %w", in.stackName, err)
	}
	switch stackDescr.StackStatus {
	case types.StackStatusCreateInProgress:
		log.Infoln()
		log.Infof("Command canceled; deleting stack %s (90m timeout).\n", in.stackName)
		log.Infoln("Press Ctrl-C again to exit immediately without waiting for CloudFormation.")
		description := fmt.Sprintf("Delete stack %s", in.stackName)
		if err := cf.deleteAndRenderStack(deleteAndRenderInput{
			ctx:         cleanupCtx,
			stackName:   in.stackName,
			description: description,
			deleteFn: func(ctx context.Context) error {
				return cf.cfnClient.DeleteAndWaitWithContext(ctx, in.stackName)
			},
			updateRenderDone: in.updateRenderDone,
		}); err != nil {
			return err
		}
		return &ErrStackDeletedOnInterrupt{stackName: in.stackName}
	case types.StackStatusUpdateInProgress:
		log.Infoln()
		log.Infof("Command canceled; canceling update for stack %s and waiting for rollback (90m timeout).\n", in.stackName)
		log.Infoln("Press Ctrl-C again to exit immediately without waiting for CloudFormation.")
		description := fmt.Sprintf("Canceling stack update %s", in.stackName)
		if err := cf.cancelUpdateAndRender(&cancelUpdateAndRenderInput{
			ctx:         cleanupCtx,
			stackName:   in.stackName,
			description: description,
			cancelUpdateFn: func(ctx context.Context) error {
				return cf.cfnClient.CancelUpdateStackWithContext(ctx, in.stackName)
			},
			updateRenderDone: in.updateRenderDone,
		}); err != nil {
			return err
		}
		return &ErrStackUpdateCanceledOnInterrupt{stackName: in.stackName}
	}
	return nil
}

func cleanupContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(parent), waitForStackTimeout)
}

type cancelUpdateAndRenderInput struct {
	ctx              context.Context
	stackName        string
	description      string
	cancelUpdateFn   func(context.Context) error
	updateRenderDone <-chan bool
}

func (cf CloudFormation) cancelUpdateAndRender(in *cancelUpdateAndRenderInput) error {
	ctx := in.ctx
	var cancel context.CancelFunc
	var stackDescr *cloudformation.StackDescription
	var err error
	if ctx != nil {
		stackDescr, err = cf.cfnClient.DescribeWithContext(ctx, in.stackName)
	} else {
		stackDescr, err = cf.cfnClient.Describe(in.stackName)
		ctx, cancel = context.WithTimeout(context.Background(), waitForStackTimeout)
		defer cancel()
	}
	if err != nil {
		return fmt.Errorf("describe stack %s: %w", in.stackName, err)
	}
	if stackDescr.ChangeSetId == nil {
		return fmt.Errorf("ChangeSetID not found for stack %s", in.stackName)

	}
	operationCtx := ctx
	g, renderCtx := errgroup.WithContext(operationCtx)
	renderer, err := cf.createChangeSetRenderer(g, renderCtx, aws.ToString(stackDescr.ChangeSetId), in.stackName, in.description, progress.RenderOptions{})
	if err != nil {
		return err
	}
	g.Go(func() error { return in.cancelUpdateFn(renderCtx) })
	g.Go(func() error {
		if in.updateRenderDone != nil {
			<-in.updateRenderDone
		}
		_, err := progress.Render(renderCtx, progress.NewTabbedFileWriter(cf.console), renderer)
		return err
	})
	if err := g.Wait(); err != nil {
		return err
	}
	if in.ctx == nil {
		return cf.errOnFailedCancelUpdateLegacy(in.stackName)
	}
	return cf.errOnFailedCancelUpdate(operationCtx, in.stackName)
}

func (cf CloudFormation) errOnFailedCancelUpdateLegacy(stackName string) error {
	stack, err := cf.cfnClient.Describe(stackName)
	if err != nil {
		return fmt.Errorf("describe stack %s: %w", stackName, err)
	}
	if stack.StackStatus != types.StackStatusUpdateRollbackComplete {
		return fmt.Errorf("stack %s did not rollback successfully and exited with status %s", stackName, stack.StackStatus)
	}
	return nil
}
func (cf CloudFormation) errOnFailedCancelUpdate(ctx context.Context, stackName string) error {
	stack, err := cf.cfnClient.DescribeWithContext(ctx, stackName)
	if err != nil {
		return fmt.Errorf("describe stack %s: %w", stackName, err)
	}
	status := string(stack.StackStatus)
	if status != string(types.StackStatusUpdateRollbackComplete) {
		return fmt.Errorf("stack %s did not rollback successfully and exited with status %s", stackName, status)
	}
	return nil
}

// ErrStackDeletedOnInterrupt means stack is deleted on interrupt.
type ErrStackDeletedOnInterrupt struct {
	stackName string
}

func (e *ErrStackDeletedOnInterrupt) Error() string {
	return fmt.Sprintf("stack %s was deleted on interrupt signal", e.stackName)
}

// ErrStackUpdateCanceledOnInterrupt means stack update is canceled on interrupt.
type ErrStackUpdateCanceledOnInterrupt struct {
	stackName string
}

func (e *ErrStackUpdateCanceledOnInterrupt) Error() string {
	return fmt.Sprintf("update for stack %s was canceled on interrupt signal", e.stackName)
}

func (cf CloudFormation) createChangeSetRenderer(group *errgroup.Group, ctx context.Context, changeSetID, stackName, description string, opts progress.RenderOptions) (progress.DynamicRenderer, error) {
	changeSet, err := cf.cfnClient.DescribeChangeSetWithContext(ctx, changeSetID, stackName)
	if err != nil {
		return nil, err
	}
	body, err := cf.cfnClient.TemplateBodyFromChangeSetWithContext(ctx, changeSetID, stackName)
	if err != nil {
		return nil, err
	}
	descriptions, err := cloudformation.ParseTemplateDescriptions(body)
	if err != nil {
		return nil, fmt.Errorf("parse cloudformation template for resource descriptions: %w", err)
	}

	streamer := stream.NewStackStreamerWithContext(ctx, cf.cfnClient, stackName, changeSet.CreationTime)
	children, err := cf.changeRenderers(changeRenderersInput{
		g:                  group,
		ctx:                ctx,
		stackName:          stackName,
		stackStreamer:      streamer,
		changes:            changeSet.Changes,
		changeSetTimestamp: changeSet.CreationTime,
		descriptions:       descriptions,
		opts:               progress.NestedRenderOptions(opts),
	})
	if err != nil {
		return nil, err
	}
	renderer := progress.ListeningChangeSetRenderer(streamer, stackName, description, children, opts)
	group.Go(func() error {
		return stream.Stream(ctx, streamer)
	})
	return renderer, nil
}

type changeRenderersInput struct {
	g                  *errgroup.Group          // Group that all goroutines belong.
	ctx                context.Context          // Context associated with the group.
	stackName          string                   // Name of the stack.
	stackStreamer      progress.StackSubscriber // Streamer for the stack where changes belong.
	changes            []types.Change           // List of changes that will be applied to the stack.
	changeSetTimestamp time.Time                // ChangeSet creation time.
	descriptions       map[string]string        // Descriptions for the logical IDs of the changes.
	opts               progress.RenderOptions   // Display options that should be applied to the changes.
}

// changeRenderers filters changes by resources that have a description and returns the appropriate progress.Renderer for each resource type.
func (cf CloudFormation) changeRenderers(in changeRenderersInput) ([]progress.Renderer, error) {
	var resources []progress.Renderer
	for _, change := range in.changes {
		change := change
		logicalID := aws.ToString(change.ResourceChange.LogicalResourceId)
		description, ok := in.descriptions[logicalID]
		if !ok {
			continue
		}
		var renderer progress.Renderer
		switch {
		case aws.ToString(change.ResourceChange.ResourceType) == envControllerResourceType:
			r, err := cf.createEnvControllerRenderer(&envControllerRendererInput{
				g:                 in.g,
				ctx:               in.ctx,
				workloadStackName: in.stackName,
				workloadTimestamp: in.changeSetTimestamp,
				change:            &change,
				description:       description,
				serviceStack:      in.stackStreamer,
				renderOpts:        in.opts,
			})
			if err != nil {
				return nil, err
			}
			renderer = r
		case aws.ToString(change.ResourceChange.ResourceType) == ecsServiceResourceType:
			renderer = progress.ListeningECSServiceResourceRenderer(progress.ECSServiceRendererCfg{
				Streamer:    in.stackStreamer,
				ECSClient:   cf.ecsClient,
				CWClient:    cf.cwClient,
				LogicalID:   logicalID,
				Description: description,
			},
				progress.ECSServiceRendererOpts{
					Group:      in.g,
					Ctx:        in.ctx,
					RenderOpts: in.opts,
				})
		case change.ResourceChange.ChangeSetId != nil:
			// The resource change is a nested stack.
			changeSetID := aws.ToString(change.ResourceChange.ChangeSetId)
			stackName := parseStackNameFromARN(aws.ToString(change.ResourceChange.PhysicalResourceId))

			r, err := cf.createChangeSetRenderer(in.g, in.ctx, changeSetID, stackName, description, in.opts)
			if err != nil {
				return nil, err
			}
			renderer = r
		default:
			renderer = progress.ListeningResourceRenderer(in.stackStreamer, logicalID, description, progress.ResourceRendererOpts{
				RenderOpts: in.opts,
			})
		}
		resources = append(resources, renderer)
	}
	return resources, nil
}

type envControllerRendererInput struct {
	g                 *errgroup.Group
	ctx               context.Context
	workloadStackName string
	workloadTimestamp time.Time
	change            *types.Change
	description       string
	serviceStack      progress.StackSubscriber
	renderOpts        progress.RenderOptions
}

func (cf CloudFormation) createEnvControllerRenderer(in *envControllerRendererInput) (progress.DynamicRenderer, error) {
	workload, err := cf.cfnClient.DescribeWithContext(in.ctx, in.workloadStackName)
	if err != nil {
		return nil, err
	}
	envStackName := fmt.Sprintf("%s-%s", parseAppNameFromTags(workload.Tags), parseEnvNameFromTags(workload.Tags))
	body, err := cf.cfnClient.TemplateBodyWithContext(in.ctx, envStackName)
	if err != nil {
		return nil, err
	}
	envResourceDescriptions, err := cloudformation.ParseTemplateDescriptions(body)
	if err != nil {
		return nil, fmt.Errorf("parse cloudformation template for resource descriptions: %w", err)
	}
	envStreamer := stream.NewStackStreamerWithContext(in.ctx, cf.cfnClient, envStackName, in.workloadTimestamp)
	ctx, cancel := context.WithCancel(in.ctx)
	in.g.Go(func() error {
		if err := stream.Stream(ctx, envStreamer); err != nil {
			if errors.Is(err, context.Canceled) {
				// The stack streamer was canceled on purposed, do not return an error.
				// This occurs if we detect that the environment stack has no updates.
				return nil
			}
			return err
		}
		return nil
	})
	return progress.ListeningEnvControllerRenderer(progress.EnvControllerConfig{
		Description:     in.description,
		RenderOpts:      in.renderOpts,
		ActionStreamer:  in.serviceStack,
		ActionLogicalID: aws.ToString(in.change.ResourceChange.LogicalResourceId),
		EnvStreamer:     envStreamer,
		CancelEnvStream: cancel,
		EnvStackName:    envStackName,
		EnvResources:    envResourceDescriptions,
	}), nil
}

type renderStackInput struct {
	group *errgroup.Group // Group of go routines.
	ctx   context.Context

	// Stack metadata.
	stackName      string            // Name of the stack.
	stackID        string            // ID of the stack.
	description    string            // Descriptive text for the stack mutation.
	descriptionFor map[string]string // Descriptive text for each resource in the stack.
	startTime      time.Time         // Timestamp for when the stack mutation started.
}

func (cf CloudFormation) stackRenderer(ctx context.Context, in renderStackInput) progress.DynamicRenderer {
	streamer := stream.NewStackStreamer(cf.cfnClient, in.stackID, in.startTime)
	if in.ctx != nil {
		streamer = stream.NewStackStreamerWithContext(ctx, cf.cfnClient, in.stackID, in.startTime)
	}
	renderer := progress.ListeningStackRenderer(streamer, in.stackName, in.description, in.descriptionFor, progress.RenderOptions{})
	in.group.Go(func() error {
		return stream.Stream(ctx, streamer)
	})
	return renderer
}

type deleteAndRenderInput struct {
	ctx              context.Context
	stackName        string
	description      string
	deleteFn         func(context.Context) error
	updateRenderDone <-chan bool
}

func (cf CloudFormation) deleteAndRenderStack(in deleteAndRenderInput) error {
	ctx := in.ctx
	var body string
	var err error
	if ctx != nil {
		body, err = cf.cfnClient.TemplateBodyWithContext(ctx, in.stackName)
	} else {
		body, err = cf.cfnClient.TemplateBody(in.stackName)
	}
	if err != nil {
		if !errors.As(err, &errNotFound) {
			return fmt.Errorf("get template body of stack %q: %w", in.stackName, err)
		}
		return nil // stack already deleted.
	}
	descriptionFor, err := cloudformation.ParseTemplateDescriptions(body)
	if err != nil {
		return fmt.Errorf("parse resource descriptions in template of stack %q: %w", in.stackName, err)
	}

	var stack *cloudformation.StackDescription
	if ctx != nil {
		stack, err = cf.cfnClient.DescribeWithContext(ctx, in.stackName)
	} else {
		stack, err = cf.cfnClient.Describe(in.stackName)
	}
	if err != nil {
		if !errors.As(err, &errNotFound) {
			return fmt.Errorf("retrieve the stack ID for stack %q: %w", in.stackName, err)
		}
		return nil // stack already deleted.
	}

	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), waitForStackTimeout)
		defer cancel()
	}
	g, ctx := errgroup.WithContext(ctx)
	now := time.Now()
	g.Go(func() error { return in.deleteFn(ctx) })
	renderer := cf.stackRenderer(ctx, renderStackInput{
		group:          g,
		ctx:            in.ctx,
		stackID:        aws.ToString(stack.StackId),
		stackName:      in.stackName,
		description:    in.description,
		descriptionFor: descriptionFor,
		startTime:      now,
	})
	g.Go(func() error {
		if in.updateRenderDone != nil {
			<-in.updateRenderDone
		}
		w := progress.NewTabbedFileWriter(cf.console)
		nl, err := progress.Render(ctx, w, renderer)
		if err != nil {
			return fmt.Errorf("render stack %q progress: %w", in.stackName, err)
		}
		_, err = progress.EraseAndRender(w, progress.LineRenderer(log.Ssuccess(in.description), 0), nl)
		if err != nil {
			return fmt.Errorf("erase and render stack %q progress: %w", in.stackName, err)
		}
		return nil
	})
	if err := g.Wait(); err != nil {
		if !errors.As(err, &errNotFound) {
			return err
		}
	}
	return nil
}

type errFailedService struct {
	stackName    string
	resourceType string
	status       string
}

func (e *errFailedService) RecommendActions() string {
	if e.resourceType == "AWS::AppRunner::Service" {
		return fmt.Sprintf("You may fix the error by updating the service code or the manifest configuration.\n"+
			"You can then retry deploying your service by running %s.", color.HighlightCode("copilot svc deploy"))
	}
	return ""
}
func (e *errFailedService) Error() string {
	return fmt.Sprintf("stack %s did not complete successfully and exited with status %s", e.stackName, e.status)
}

func (cf CloudFormation) errOnFailedStack(ctx context.Context, stackName string) error {
	stack, err := cf.cfnClient.DescribeWithContext(ctx, stackName)
	if err != nil {
		return err
	}
	status := string(stack.StackStatus)
	if cloudformation.StackStatus(status).IsFailure() {
		events, _ := cf.cfnClient.ErrorEventsWithContext(ctx, stackName)
		var failedResourceType string
		if len(events) > 0 {
			failedResourceType = aws.ToString(events[0].ResourceType)
		}
		return &errFailedService{
			stackName:    stackName,
			resourceType: failedResourceType,
			status:       status,
		}
	}
	return nil
}

func toStack(config StackConfiguration) (*cloudformation.Stack, error) {
	template, err := config.Template()
	if err != nil {
		return nil, err
	}
	stack := cloudformation.NewStack(config.StackName(), template)
	params, err := config.Parameters()
	if err != nil {
		return nil, err
	}
	stack.Parameters = flattenParameters(params)
	stack.Tags = flattenTags(config.Tags())
	return stack, nil
}

func toStackFromS3(config StackConfiguration, s3url string) (*cloudformation.Stack, error) {
	stack := cloudformation.NewStackWithURL(config.StackName(), s3url)
	var err error
	params, err := config.Parameters()
	if err != nil {
		return nil, err
	}
	stack.Parameters = flattenParameters(params)
	stack.Tags = flattenTags(config.Tags())
	return stack, nil
}

func toMap(tags []types.Tag) map[string]string {
	m := make(map[string]string)
	for _, t := range tags {
		m[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	return m
}

func toMapPtr(tags []*types.Tag) map[string]string {
	m := make(map[string]string)
	for _, t := range tags {
		if t == nil {
			continue
		}
		m[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	return m
}

func flattenParameters(params []*types.Parameter) []types.Parameter {
	flat := make([]types.Parameter, 0, len(params))
	for _, param := range params {
		if param == nil {
			continue
		}
		flat = append(flat, *param)
	}
	return flat
}

func flattenTags(tags []*types.Tag) []types.Tag {
	flat := make([]types.Tag, 0, len(tags))
	for _, tag := range tags {
		if tag == nil {
			continue
		}
		flat = append(flat, *tag)
	}
	return flat
}

// parseStackNameFromARN retrieves "my-nested-stack" from an input like:
// arn:aws:cloudformation:us-west-2:123456789012:stack/my-nested-stack/d0a825a0-e4cd-xmpl-b9fb-061c69e99205
func parseStackNameFromARN(stackARN string) string {
	return strings.Split(stackARN, "/")[1]
}

func parseAppNameFromTags(tags []types.Tag) string {
	for _, t := range tags {
		if aws.ToString(t.Key) == deploy.AppTagKey {
			return aws.ToString(t.Value)
		}
	}
	return ""
}

func parseEnvNameFromTags(tags []types.Tag) string {
	for _, t := range tags {
		if aws.ToString(t.Key) == deploy.EnvTagKey {
			return aws.ToString(t.Value)
		}
	}
	return ""
}

func stopSpinner(spinner *progress.Spinner, err error, label string) {
	if err == nil {
		spinner.Stop(log.Ssuccessf("%s\n", label))
		return
	}
	var existsErr *cloudformation.ErrStackAlreadyExists
	if errors.As(err, &existsErr) {
		spinner.Stop(log.Ssuccessf("%s\n", label))
		return
	}
	spinner.Stop(log.Serrorf("%s\n", label))
}
