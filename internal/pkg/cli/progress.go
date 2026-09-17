// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// Copyright APROINT, s.r.o. in modifications to this fork.
// SPDX-License-Identifier: Apache-2.0

package cli

// progress is the interface to inform the user that a long operation is taking place.
type progress interface {
	// Start starts displaying progress with a label.
	Start(label string)
	// Stop ends displaying progress with a label.
	Stop(label string)
}
