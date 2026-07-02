// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0
"use strict";

const js = require("@eslint/js");
const globals = require("globals");
const promise = require("eslint-plugin-promise");

module.exports = [
  {
    ignores: ["coverage/**", "node_modules/**"],
  },
  {
    files: ["lib/**/*.js", "test/**/*.js", "scripts/**/*.js", "*.config.js"],
    languageOptions: {
      ecmaVersion: "latest",
      sourceType: "commonjs",
      globals: {
        ...globals.node,
        ...globals.es2024,
        ...globals.vitest,
      },
    },
    plugins: {
      promise,
    },
    rules: {
      ...js.configs.recommended.rules,
      ...promise.configs.recommended.rules,
      "no-case-declarations": "off",
      "no-global-assign": "off",
      "no-unsafe-negation": "off",
      "no-useless-assignment": "off",
      "no-useless-escape": "off",
      "no-unused-vars": "off",
      "preserve-caught-error": "off",
      "promise/always-return": "off",
      "promise/param-names": "off",
    },
  },
];
