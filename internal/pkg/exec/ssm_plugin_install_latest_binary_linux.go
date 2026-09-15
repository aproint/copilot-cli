// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package exec provides an interface to execute certain commands.
package exec

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// InstallLatestBinary installs the latest ssm plugin.
func (s SSMPluginCommand) InstallLatestBinary(ctx context.Context) error {
	if s.tempDir == "" {
		dir, err := os.MkdirTemp("", "ssmplugin")
		if err != nil {
			return fmt.Errorf("create a temporary directory: %w", err)
		}
		defer os.RemoveAll(dir)
		s.tempDir = dir
	}
	isUbuntu, err := s.isUbuntu(ctx)
	if err != nil {
		return err
	}
	if isUbuntu {
		return s.installUbuntuBinary(ctx)
	}
	return s.installLinuxBinary(ctx)
}

func (s SSMPluginCommand) isUbuntu(ctx context.Context) (bool, error) {
	if err := s.runner.Run(ctx, "cat", []string{"/etc/os-release"}, Stdout(&s.linuxDistVersionBuffer)); err != nil {
		return false, fmt.Errorf("run cat /etc/os-release: %w", err)
	}
	/*
	   Example output of /etc/os-release:
	   "ID=ubuntu"
	   "ID_LIKE="debian""
	*/
	scanner := bufio.NewScanner(strings.NewReader(s.linuxDistVersionBuffer.String()))
	for scanner.Scan() {
		line := scanner.Text()
		keyValuePair := strings.SplitN(strings.Trim(strings.TrimSpace(line), `"'`), "=", 2) // Remove potential quotes and newlines.
		if len(keyValuePair) != 2 {
			continue
		}
		key := keyValuePair[0]
		value := strings.Trim(strings.TrimSpace(keyValuePair[1]), `"'`) // Remove potential quotes and newlines.
		if (key == "ID" || key == "ID_LIKE") && strings.Contains(strings.ToLower(value), "ubuntu") {
			return true, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return false, err
	}
	return false, nil
}

func (s SSMPluginCommand) installLinuxBinary(ctx context.Context) error {
	if err := download(s.http, filepath.Join(s.tempDir, "session-manager-plugin.rpm"), linuxSSMPluginBinaryURL); err != nil {
		return fmt.Errorf("download ssm plugin: %w", err)
	}
	if err := s.runner.Run(ctx, "sudo", []string{"yum", "install", "-y",
		filepath.Join(s.tempDir, "session-manager-plugin.rpm")}); err != nil {
		return err
	}
	return nil
}

func (s SSMPluginCommand) installUbuntuBinary(ctx context.Context) error {
	if err := download(s.http, filepath.Join(s.tempDir, "session-manager-plugin.deb"), ubuntuSSMPluginBinaryURL); err != nil {
		return fmt.Errorf("download ssm plugin: %w", err)
	}
	if err := s.runner.Run(ctx, "sudo", []string{"dpkg", "-i",
		filepath.Join(s.tempDir, "session-manager-plugin.deb")}); err != nil {
		return err
	}
	return nil
}
