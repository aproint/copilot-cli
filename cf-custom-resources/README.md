<!--
Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
SPDX-License-Identifier: Apache-2.0
-->

# Custom Resources

The `package` script minifies each file in `lib/` with Terser and writes one
same-named JavaScript artifact per source file to
`internal/pkg/template/templates/custom-resources/`.

The Terser migration keeps the existing one-source-file-to-one-artifact layout
that the Go templates expect. As of the APR-24 refresh, packaging produces 14
custom-resource artifacts totaling about 62 KB before the generated files are
removed by `make package-custom-resources-clean`.
