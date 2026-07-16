import pkg from "../package.json" with { type: "json" };

// A statically analyzable JSON import (not a runtime require()) so
// bundlers — including `bun build --compile`, which produces a
// standalone binary with a virtual filesystem that doesn't preserve real
// relative disk paths — can inline the value at build time instead of
// trying to resolve it on disk at runtime.
export const VERSION = (pkg as { version: string }).version;
