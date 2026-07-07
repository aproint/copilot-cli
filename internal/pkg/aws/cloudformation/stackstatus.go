// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package cloudformation

import (
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
)

var (
	successStackStatuses = []string{
		string(types.StackStatusCreateComplete),
		string(types.StackStatusDeleteComplete),
		string(types.StackStatusUpdateComplete),
		string(types.StackStatusUpdateCompleteCleanupInProgress),
		string(types.StackStatusImportComplete),
	}

	failureStackStatuses = []string{
		string(types.StackStatusCreateFailed),
		string(types.StackStatusDeleteFailed),
		string(types.ResourceStatusUpdateFailed),
		string(types.StackStatusRollbackInProgress),
		string(types.StackStatusRollbackComplete),
		string(types.StackStatusRollbackFailed),
		string(types.StackStatusUpdateRollbackComplete),
		string(types.StackStatusUpdateRollbackCompleteCleanupInProgress),
		string(types.StackStatusUpdateRollbackInProgress),
		string(types.StackStatusUpdateRollbackFailed),
		string(types.ResourceStatusImportRollbackInProgress),
		string(types.ResourceStatusImportRollbackFailed),
	}
)

// StackStatus represents the status of a stack.
type StackStatus string

// requiresCleanup returns true if the stack was created, but failed and should be deleted.
func (ss StackStatus) requiresCleanup() bool {
	return string(types.StackStatusRollbackComplete) == string(ss) || string(types.StackStatusRollbackFailed) == string(ss)
}

// InProgress returns true if the stack is currently being updated.
func (ss StackStatus) InProgress() bool {
	return strings.HasSuffix(string(ss), "IN_PROGRESS")
}

// UpsertInProgress returns true if the resource is updating or being created.
func (ss StackStatus) UpsertInProgress() bool {
	return ss == StackStatus(types.StackStatusCreateInProgress) || ss == StackStatus(types.StackStatusUpdateInProgress)
}

// IsSuccess returns true if the resource mutated successfully.
func (ss StackStatus) IsSuccess() bool {
	for _, success := range successStackStatuses {
		if string(ss) == success {
			return true
		}
	}
	return false
}

// IsFailure returns true if the resource failed to mutate.
func (ss StackStatus) IsFailure() bool {
	for _, failure := range failureStackStatuses {
		if string(ss) == failure {
			return true
		}
	}
	return false
}

// String implements the fmt.Stringer interface.
func (ss StackStatus) String() string {
	return string(ss)
}
