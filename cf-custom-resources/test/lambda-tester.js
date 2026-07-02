// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0
"use strict";

function LambdaTester(handler) {
  let lambdaEvent = {};
  let lambdaContext = {};

  return {
    event(event) {
      lambdaEvent = event;
      return this;
    },

    context(context) {
      lambdaContext = context;
      return this;
    },

    async expectResolve(assertion) {
      const result = await handler(lambdaEvent, lambdaContext);
      if (assertion) {
        await assertion(result);
      }
    },

    async expectReject(assertion) {
      try {
        await handler(lambdaEvent, lambdaContext);
      } catch (err) {
        if (assertion) {
          await assertion(err);
        }
        return;
      }

      throw new Error("Expected Lambda handler to reject");
    },
  };
}

LambdaTester.noVersionCheck = () => LambdaTester;

module.exports = LambdaTester;
