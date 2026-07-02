// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0
"use strict";

const resetESModules = global.vi.resetModules.bind(global.vi);

global.vi.resetModules = () => {
  resetESModules();
  for (const modulePath of Object.keys(require.cache)) {
    if (modulePath.includes("/cf-custom-resources/lib/")) {
      delete require.cache[modulePath];
    }
  }
};
