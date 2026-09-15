// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"context"
	"strings"

	"github.com/aproint/copilot-cli/internal/pkg/exec"
)

func describeGitChanges(ctx context.Context, r execRunner) (string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := r.Run(ctx, "git", []string{"describe", "--always"}, exec.Stdout(&stdout), exec.Stderr(&stderr)); err != nil {
		return "", err
	}
	// NOTE: `git describe` output bytes includes a `\n` character, so we trim it out.
	return strings.TrimSpace(stdout.String()), nil
}

func hasUncommitedGitChanges(ctx context.Context, r execRunner) (bool, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := r.Run(ctx, "git", []string{"status", "--porcelain"}, exec.Stdout(&stdout), exec.Stderr(&stderr)); err != nil {
		return false, err
	}
	return strings.TrimSpace(stdout.String()) != "", nil
}

// imageTagFromGit returns the image tag to apply in case the user is in a git repository.
// If there is a clean git commit with no local changes, then return the git commit id.
// Otherwise, returns the empty string.
func imageTagFromGit(ctx context.Context, r execRunner) string {
	commit, err := describeGitChanges(ctx, r)
	if err != nil {
		return ""
	}
	isRepoDirty, _ := hasUncommitedGitChanges(ctx, r)
	if isRepoDirty {
		return ""
	}
	return commit
}
