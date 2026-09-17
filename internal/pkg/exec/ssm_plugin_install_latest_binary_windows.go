// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// Copyright APROINT, s.r.o. in modifications to this fork.
// SPDX-License-Identifier: Apache-2.0

// Package exec provides an interface to execute certain commands.
package exec

import "context"

// InstallLatestBinary returns nil and ssm plugin needs to be installed manually.
func (s SSMPluginCommand) InstallLatestBinary(_ context.Context) error {
	return nil
}
