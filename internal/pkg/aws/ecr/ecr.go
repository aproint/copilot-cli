// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// Copyright APROINT, s.r.o. in modifications to this fork.
// SPDX-License-Identifier: Apache-2.0

// Package ecr provides a client to make API requests to Amazon EC2 Container Registry.
package ecr

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/aproint/copilot-cli/internal/pkg/term/log"
	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/ecr/types"
)

const (
	urlFmtString      = "%s.dkr.ecr.%s.amazonaws.com/%s"
	urlFmtStringForCN = "%s.dkr.ecr.%s.amazonaws.com.cn/%s"
	arnResourcePrefix = "repository/"
	batchDeleteLimit  = 100
)

type api interface {
	DescribeImages(context.Context, *ecr.DescribeImagesInput, ...func(*ecr.Options)) (*ecr.DescribeImagesOutput, error)
	GetAuthorizationToken(context.Context, *ecr.GetAuthorizationTokenInput, ...func(*ecr.Options)) (*ecr.GetAuthorizationTokenOutput, error)
	DescribeRepositories(context.Context, *ecr.DescribeRepositoriesInput, ...func(*ecr.Options)) (*ecr.DescribeRepositoriesOutput, error)
	BatchDeleteImage(context.Context, *ecr.BatchDeleteImageInput, ...func(*ecr.Options)) (*ecr.BatchDeleteImageOutput, error)
}

// ECR wraps an AWS ECR client.
type ECR struct {
	client api
}

// New returns a ECR configured against the input SDK v2 config.
func New(cfg awsv2.Config) ECR {
	return ECR{
		client: ecr.NewFromConfig(cfg),
	}
}

// Auth returns the basic authentication credentials needed to push images using ctx.
func (c ECR) Auth(ctx context.Context) (username string, password string, err error) {
	response, err := c.client.GetAuthorizationToken(ctx, &ecr.GetAuthorizationTokenInput{})

	if err != nil {
		return "", "", fmt.Errorf("get ECR auth: %w", err)
	}

	authToken, err := base64.StdEncoding.DecodeString(*response.AuthorizationData[0].AuthorizationToken)

	if err != nil {
		return "", "", fmt.Errorf("decode auth token: %w", err)
	}

	tokenStrings := strings.Split(string(authToken), ":")
	return tokenStrings[0], tokenStrings[1], nil
}

// RepositoryURI returns the ECR repository URI using ctx.
func (c ECR) RepositoryURI(ctx context.Context, name string) (string, error) {
	result, err := c.client.DescribeRepositories(ctx, &ecr.DescribeRepositoriesInput{
		RepositoryNames: []string{name},
	})

	if err != nil {
		return "", fmt.Errorf("ecr describe repository %s: %w", name, err)
	}

	foundRepositories := result.Repositories

	if len(foundRepositories) <= 0 {
		return "", errors.New("no repositories found")
	}

	repo := result.Repositories[0]

	return *repo.RepositoryUri, nil
}

// Image houses metadata for ECR repository images.
type Image struct {
	Digest string
}

func (i Image) imageIdentifier() types.ImageIdentifier {
	return types.ImageIdentifier{
		ImageDigest: awsv2.String(i.Digest),
	}
}

// ListImages returns images in a repository using ctx for every page.
func (c ECR) ListImages(ctx context.Context, repoName string) ([]Image, error) {
	var images []Image
	resp, err := c.client.DescribeImages(ctx, &ecr.DescribeImagesInput{
		RepositoryName: awsv2.String(repoName),
	})
	if err != nil {
		return nil, fmt.Errorf("ecr repo %s describe images: %w", repoName, err)
	}
	for _, imageDetails := range resp.ImageDetails {
		images = append(images, Image{
			Digest: awsv2.ToString(imageDetails.ImageDigest),
		})
	}
	for resp.NextToken != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		resp, err = c.client.DescribeImages(ctx, &ecr.DescribeImagesInput{
			RepositoryName: awsv2.String(repoName),
			NextToken:      resp.NextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("ecr repo %s describe images: %w", repoName, err)
		}
		for _, imageDetails := range resp.ImageDetails {
			images = append(images, Image{
				Digest: awsv2.ToString(imageDetails.ImageDigest),
			})
		}
	}
	return images, nil
}

// DeleteImages deletes images using ctx.
func (c ECR) DeleteImages(ctx context.Context, images []Image, repoName string) error {
	if len(images) == 0 {
		return nil
	}

	var imageIdentifiers []types.ImageIdentifier
	for _, image := range images {
		imageIdentifiers = append(imageIdentifiers, image.imageIdentifier())
	}
	var imageIdentifiersBatch [][]types.ImageIdentifier
	for batchDeleteLimit < len(imageIdentifiers) {
		imageIdentifiers, imageIdentifiersBatch = imageIdentifiers[batchDeleteLimit:], append(imageIdentifiersBatch, imageIdentifiers[0:batchDeleteLimit])
	}
	imageIdentifiersBatch = append(imageIdentifiersBatch, imageIdentifiers)
	for _, identifiers := range imageIdentifiersBatch {
		if err := ctx.Err(); err != nil {
			return err
		}
		resp, err := c.client.BatchDeleteImage(ctx, &ecr.BatchDeleteImageInput{
			RepositoryName: awsv2.String(repoName),
			ImageIds:       identifiers,
		})
		if resp != nil {
			for _, failure := range resp.Failures {
				log.Warningf("failed to delete %s:%s : %s %s\n", failure.ImageId.ImageDigest, failure.ImageId.ImageTag, failure.FailureCode, failure.FailureReason)
			}
		}
		if err != nil {
			return fmt.Errorf("ecr repo %s batch delete image: %w", repoName, err)
		}
	}

	return nil
}

// ClearRepository removes every image from a repository using ctx.
func (c ECR) ClearRepository(ctx context.Context, repoName string) error {
	images, err := c.ListImages(ctx, repoName)

	if err == nil {
		// TODO: add retry handling in case images are added to a repository after a call to ListImages
		return c.DeleteImages(ctx, images, repoName)
	}
	if isRepoNotFoundErr(errors.Unwrap(err)) {
		return nil
	}
	return err
}

// URIFromARN converts an ECR Repo ARN to a Repository URI
func URIFromARN(repositoryARN string) (string, error) {
	repoARN, err := arn.Parse(repositoryARN)
	if err != nil {
		return "", fmt.Errorf("parsing repository ARN %s: %w", repositoryARN, err)
	}
	urlFmtStr := urlFmtString
	if repoARN.Partition == "aws-cn" {
		urlFmtStr = urlFmtStringForCN
	}
	// Repo ARNs look like arn:aws:ecr:region:012345678910:repository/test
	// so we have to strip the repository out.
	repoName := strings.TrimPrefix(repoARN.Resource, arnResourcePrefix)
	return fmt.Sprintf(urlFmtStr,
		repoARN.AccountID,
		repoARN.Region,
		repoName), nil
}

func isRepoNotFoundErr(err error) bool {
	var repoNotFound *types.RepositoryNotFoundException
	return errors.As(err, &repoNotFound)
}
