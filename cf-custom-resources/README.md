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

## Sandbox AWS validation checklist

Before promoting custom-resource runtime changes, validate the generated
Lambdas in a sandbox account with a new app and environment:

- Domains: deploy a load balanced web service with an HTTPS alias and confirm
  the custom domain and listener rule custom resources complete successfully.
- Certificates: deploy environment and workload flows that request and validate
  ACM certificates, including DNS delegation when available.
- ALB rules: deploy services with multiple HTTP and HTTPS path rules and confirm
  rule priority allocation succeeds on create and update.
- Bucket cleanup: enable ELB access logs, delete the environment, and confirm
  the access log bucket cleaner removes objects during stack deletion.
- App Runner: deploy a request-driven web service with a custom domain and
  confirm App Runner domain association and cleanup complete successfully.
- ECS flows: deploy and update load balanced, backend, worker, and scheduled
  job workloads so env controller, desired count, autoscaling, and state machine
  custom resources run under the generated Node.js runtime.
