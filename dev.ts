#!/usr/bin/env -S bun --install=force
import { Register } from "@jpillora/take";
import genTypes from "./.dev/gen-types.ts";
import checkTypes from "./.dev/check-types.ts";
import frontend from "./.dev/frontend.ts";
import build from "./.dev/build.ts";
import test from "./.dev/test.ts";
import ci from "./.dev/ci.ts";
import release from "./.dev/release.ts";
import e2e from "./.dev/e2e.ts";

process.chdir(import.meta.dir);
await Register(genTypes, checkTypes, frontend, build, test, ci, release, e2e);
