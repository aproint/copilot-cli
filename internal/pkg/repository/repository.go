// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// Copyright APROINT, s.r.o. in modifications to this fork.
// SPDX-License-Identifier: Apache-2.0

// Package repository provides support for building and pushing images to a repository.
package repository

import (
	"context"
	"fmt"
	"io"

	"github.com/aproint/copilot-cli/internal/pkg/exec"

	"github.com/aproint/copilot-cli/internal/pkg/docker/dockerengine"
)

// ContainerLoginBuildPusher provides support for logging in to repositories, building images and pushing images to repositories.
type ContainerLoginBuildPusher interface {
	Build(ctx context.Context, args *dockerengine.BuildArguments, w io.Writer) error
	Login(ctx context.Context, uri, username, password string) error
	Push(ctx context.Context, uri string, w io.Writer, tags ...string) (digest string, err error)
	IsEcrCredentialHelperEnabled(uri string) bool
}

// Registry gets information of repositories.
type Registry interface {
	RepositoryURI(ctx context.Context, name string) (string, error)
	Auth(ctx context.Context) (string, string, error)
}

// Repository builds and pushes images to a repository.
type Repository struct {
	name     string
	registry Registry
	uri      string
	docker   ContainerLoginBuildPusher
}

// New instantiates a new Repository.
func New(registry Registry, name string) *Repository {
	return &Repository{
		name:     name,
		registry: registry,
		docker:   dockerengine.New(exec.NewCmd()),
	}
}

// NewWithURI instantiates a new Repository with uri being set.
func NewWithURI(registry Registry, name, uri string) *Repository {
	return &Repository{
		name:     name,
		registry: registry,
		uri:      uri,
		docker:   dockerengine.New(exec.NewCmd()),
	}
}

// Build build the image from Dockerfile
func (r *Repository) Build(ctx context.Context, args *dockerengine.BuildArguments, w io.Writer) (digest string, err error) {
	if err := r.docker.Build(ctx, args, w); err != nil {
		return "", fmt.Errorf("build from Dockerfile at %s: %w", args.Dockerfile, err)
	}
	// digest will be an empty string here
	return digest, nil
}

// BuildAndPush builds the image from Dockerfile and pushes it to the repository with tags.
func (r *Repository) BuildAndPush(ctx context.Context, args *dockerengine.BuildArguments, w io.Writer) (digest string, err error) {
	if args.URI == "" {
		uri, err := r.repositoryURI(ctx)
		if err != nil {
			return "", err
		}
		args.URI = uri
	}
	if err := r.docker.Build(ctx, args, w); err != nil {
		return "", fmt.Errorf("build Dockerfile at %s: %w", args.Dockerfile, err)
	}

	digest, err = r.docker.Push(ctx, args.URI, w, args.Tags...)
	if err != nil {
		return "", fmt.Errorf("push to repo %s: %w", r.name, err)
	}
	return digest, nil
}

func (r *Repository) repositoryURI(ctx context.Context) (string, error) {
	if r.uri != "" {
		return r.uri, nil
	}
	uri, err := r.registry.RepositoryURI(ctx, r.name)
	if err != nil {
		return "", fmt.Errorf("get repository URI: %w", err)
	}
	r.uri = uri
	return uri, nil
}

// Login authenticates with an ECR registry using ctx.
func (r *Repository) Login(ctx context.Context) (string, error) {
	uri, err := r.repositoryURI(ctx)
	if err != nil {
		return "", fmt.Errorf("retrieve URI for repository: %w", err)
	}
	if !r.docker.IsEcrCredentialHelperEnabled(uri) {
		username, password, err := r.registry.Auth(ctx)
		if err != nil {
			return "", fmt.Errorf("get auth: %w", err)
		}

		if err := r.docker.Login(ctx, uri, username, password); err != nil {
			return "", fmt.Errorf("docker login %s: %w", uri, err)
		}
	}
	return uri, nil
}
