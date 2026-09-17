// Copyright APROINT, s.r.o.
// SPDX-License-Identifier: Apache-2.0
"use strict";

const resetESModules = global.vi.resetModules.bind(global.vi);

global.vi.resetModules = () => {
  resetESModules();
  for (const modulePath of Object.keys(require.cache)) {
    const normalizedModulePath = modulePath.replace(/\\/g, "/");
    if (normalizedModulePath.includes("/cf-custom-resources/lib/")) {
      delete require.cache[modulePath];
    }
  }
};
