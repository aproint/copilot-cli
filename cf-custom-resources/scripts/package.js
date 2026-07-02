// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0
"use strict";

const fs = require("fs/promises");
const path = require("path");
const { minify } = require("terser");

const sourceDir = path.join(__dirname, "..", "lib");
const destinationDir = path.join(__dirname, "..", "..", "internal", "pkg", "template", "templates", "custom-resources");

async function packageCustomResources() {
  await fs.mkdir(destinationDir, { recursive: true });
  await cleanDestination();

  const entries = await fs.readdir(sourceDir, { withFileTypes: true });
  const jsFiles = entries
    .filter((entry) => entry.isFile() && entry.name.endsWith(".js"))
    .map((entry) => entry.name)
    .sort();

  await Promise.all(jsFiles.map(async (fileName) => {
    const sourcePath = path.join(sourceDir, fileName);
    const destinationPath = path.join(destinationDir, fileName);
    const source = await fs.readFile(sourcePath, "utf8");
    const result = await minify({ [fileName]: source }, {
      compress: true,
      mangle: true,
      ecma: 2020,
    });

    if (!result.code) {
      throw new Error(`Terser did not produce output for ${fileName}`);
    }

    await fs.writeFile(destinationPath, `${result.code}\n`);
  }));
}

async function cleanDestination() {
  const entries = await fs.readdir(destinationDir, { withFileTypes: true });
  await Promise.all(entries
    .filter((entry) => entry.isFile() && entry.name.endsWith(".js"))
    .map((entry) => fs.rm(path.join(destinationDir, entry.name))));
}

packageCustomResources().catch((err) => {
  console.error(err);
  process.exit(1);
});
