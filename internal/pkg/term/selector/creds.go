// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package selector

import (
	"context"
	"fmt"
	"strings"

	"github.com/aproint/copilot-cli/internal/pkg/aws/sessions"
	"github.com/aproint/copilot-cli/internal/pkg/term/prompt"
	"github.com/aws/aws-sdk-go-v2/aws"
)

const (
	tempCredsOption       = "Enter temporary credentials"
	accessKeyIDPrompt     = "What's your AWS Access Key ID?"
	secretAccessKeyPrompt = "What's your AWS Secret Access Key?"
	sessionTokenPrompt    = "What's your AWS Session Token?"
)

// Names wraps the method that returns a list of names.
type Names interface {
	Names() []string
}

// SessionProvider wraps the methods to create AWS sessions.
type SessionProvider interface {
	DefaultConfig(ctx context.Context) (aws.Config, error)
	ConfigFromProfile(ctx context.Context, name string) (aws.Config, error)
	ConfigFromStaticCreds(accessKeyID, secretAccessKey, sessionToken string) (aws.Config, error)
}

// CredsSelect prompts users for credentials.
type CredsSelect struct {
	Prompt  Prompter
	Profile Names
	Session SessionProvider
}

// Creds prompts users to choose either use temporary credentials or choose from one of their existing AWS named profiles.
func (s *CredsSelect) Creds(msg, help string) (aws.Config, error) {
	profileFrom := make(map[string]string)
	options := []string{tempCredsOption}
	for _, name := range s.Profile.Names() {
		pretty := fmt.Sprintf("[profile %s]", name)
		options = append(options, pretty)
		profileFrom[pretty] = name
	}

	selected, err := s.Prompt.SelectOne(
		msg,
		help,
		options,
		prompt.WithFinalMessage("Credential source:"))
	if err != nil {
		return aws.Config{}, fmt.Errorf("select credential source: %w", err)
	}

	if selected == tempCredsOption {
		return s.askTempCreds()
	}
	cfg, err := s.Session.ConfigFromProfile(context.Background(), profileFrom[selected])
	if err != nil {
		return aws.Config{}, fmt.Errorf("create config from profile %s: %w", profileFrom[selected], err)
	}
	return cfg, nil
}

func (s *CredsSelect) askTempCreds() (aws.Config, error) {
	defaultAccessKey, defaultSecretAccessKey, defaultSessToken := defaultCreds(s.Session)

	accessKeyID, err := s.askWithMaskedDefault(accessKeyIDPrompt, defaultAccessKey, prompt.RequireNonEmpty, prompt.WithFinalMessage("AWS Access Key ID:"))
	if err != nil {
		return aws.Config{}, fmt.Errorf("get access key id: %w", err)
	}
	secretAccessKey, err := s.askWithMaskedDefault(secretAccessKeyPrompt, defaultSecretAccessKey, prompt.RequireNonEmpty, prompt.WithFinalMessage("AWS Secret Access Key:"))
	if err != nil {
		return aws.Config{}, fmt.Errorf("get secret access key: %w", err)
	}
	sessionToken, err := s.askWithMaskedDefault(sessionTokenPrompt, defaultSessToken, nil, prompt.WithFinalMessage("AWS Session Token:"))
	if err != nil {
		return aws.Config{}, fmt.Errorf("get session token: %w", err)
	}

	cfg, err := s.Session.ConfigFromStaticCreds(accessKeyID, secretAccessKey, sessionToken)
	if err != nil {
		return aws.Config{}, fmt.Errorf("create config from temporary credentials: %w", err)
	}
	return cfg, nil
}

func (s *CredsSelect) askWithMaskedDefault(msg, defaultValue string, f prompt.ValidatorFunc, opts ...prompt.PromptConfig) (string, error) {
	accessKeyId, err := s.Prompt.Get(msg, "", f, append([]prompt.PromptConfig{prompt.WithDefaultInput(mask(defaultValue))}, opts...)...)
	if err != nil {
		return "", err
	}
	if accessKeyId == mask(defaultValue) {
		// Return the original default instead of the masked value.
		return defaultValue, nil
	}
	return accessKeyId, nil
}

// defaultCreds returns the credential values from the default session.
// If an error occurs, returns empty strings.
func defaultCreds(session SessionProvider) (accessKeyID, secretAccessKey, sessionToken string) {
	// If we cannot retrieve default creds, return empty credentials as default instead of an error.
	defaultConfig, err := session.DefaultConfig(context.Background())
	if err != nil {
		return
	}
	v, err := sessions.V2Creds(context.Background(), defaultConfig)
	if err != nil {
		return
	}
	return v.AccessKeyID, v.SecretAccessKey, v.SessionToken
}

// mask hides the value of s with "*"s except the last 4 characters.
// Taken from the AWS CLI, see:
// https://github.com/aws/aws-cli/blob/4ff0cbacbac69a21d4dd701921fe0759cf7852ed/awscli/customizations/configure/__init__.py#L38-L42
// TODO(efekarakus): Move the masking logic to be part of the prompt package.
func mask(s string) string {
	if s == "" {
		return ""
	}

	hint := s
	if len(hint) >= 4 {
		hint = hint[len(hint)-4:]
	}
	return fmt.Sprintf("%s%s", strings.Repeat("*", 16), hint)
}
