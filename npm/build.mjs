#!/usr/bin/env node
// Assembles the npm packages from compiled Go binaries.
//
//   node npm/build.mjs --version 1.2.3 --dist dist --out dist-npm
//
// Produces one package per platform, each carrying a single binary, plus the
// CLI package that depends on all of them as optionalDependencies. npm then
// installs exactly one binary per machine, resolved by the `os`/`cpu` fields —
// no postinstall script, so it works under --ignore-scripts.

import { mkdir, writeFile, copyFile, rm } from "node:fs/promises";
import { existsSync } from "node:fs";
import path from "node:path";

import { TARGETS, CLI_PACKAGE, binaryName } from "./cli/lib/resolve.js";

function arg(name, fallback) {
  const i = process.argv.indexOf(`--${name}`);
  if (i !== -1 && process.argv[i + 1]) return process.argv[i + 1];
  if (fallback !== undefined) return fallback;
  throw new Error(`missing required argument --${name}`);
}

const version = arg("version");
const distDir = path.resolve(arg("dist", "dist"));
const outDir = path.resolve(arg("out", "dist-npm"));
const repoRoot = path.resolve(path.dirname(new URL(import.meta.url).pathname), "..");

// A malformed version is rejected here rather than by the registry, so a bad CI
// run fails at build time instead of half-publishing.
if (!/^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$/.test(version)) {
  throw new Error(`--version must be semver, got "${version}"`);
}

const common = {
  version,
  license: "MIT",
  repository: {
    type: "git",
    url: "git+https://github.com/alvaroofernaandez/aval.git",
  },
  homepage: "https://github.com/alvaroofernaandez/aval#readme",
  publishConfig: { access: "public" },
};

await rm(outDir, { recursive: true, force: true });
await mkdir(outDir, { recursive: true });

const built = [];

for (const target of TARGETS) {
  const file = binaryName(target.platform);
  const source = path.join(distDir, `aval-${target.goos}-${target.goarch}${target.platform === "win32" ? ".exe" : ""}`);

  // All or nothing: a CLI published without one of its binaries is worse than
  // no release.
  if (!existsSync(source)) throw new Error(`missing binary for ${target.pkg}: ${source}`);

  const dir = path.join(outDir, target.pkg.replace(/^@[^/]+\//, ""));
  await mkdir(path.join(dir, "bin"), { recursive: true });
  await copyFile(source, path.join(dir, "bin", file));

  await writeFile(
    path.join(dir, "package.json"),
    JSON.stringify(
      {
        name: target.pkg,
        ...common,
        description: `aval binary for ${target.platform} ${target.arch}`,
        // These two fields are the whole mechanism: npm consults them and skips
        // the package entirely on any other machine.
        os: [target.platform],
        cpu: [target.arch],
        files: ["bin"],
      },
      null,
      2,
    ) + "\n",
  );

  built.push(target);
  console.log(`built ${target.pkg}`);
}

// --- the CLI package ---------------------------------------------------------
const cliDir = path.join(outDir, "cli");
await mkdir(path.join(cliDir, "bin"), { recursive: true });
await mkdir(path.join(cliDir, "lib"), { recursive: true });

await copyFile(
  path.join(repoRoot, "npm/cli/bin/aval.js"),
  path.join(cliDir, "bin/aval.js"),
);
await copyFile(
  path.join(repoRoot, "npm/cli/lib/resolve.js"),
  path.join(cliDir, "lib/resolve.js"),
);
await copyFile(path.join(repoRoot, "npm/cli/README.md"), path.join(cliDir, "README.md"));

await writeFile(
  path.join(cliDir, "package.json"),
  JSON.stringify(
    {
      name: CLI_PACKAGE,
      ...common,
      description:
        "aval — a personal agent harness: terminal, AI agents and evidence. macOS, Linux and Windows",
      keywords: ["aval", "harness", "dotfiles", "installer", "tui", "tmux", "yazi", "ghostty", "claude-code"],
      type: "module",
      bin: { "aval": "bin/aval.js" },
      files: ["bin", "lib", "README.md"],
      engines: { node: ">=18" },
      // optionalDependencies, not dependencies: npm must be free to skip the
      // six that do not match this machine. As hard dependencies the install
      // would fail everywhere, since five of them can never be satisfied.
      optionalDependencies: Object.fromEntries(built.map((t) => [t.pkg, version])),
    },
    null,
    2,
  ) + "\n",
);

console.log(`built ${CLI_PACKAGE} with ${built.length} optional binaries`);

// A manifest of what to publish, so the workflow does not have to re-derive it.
await writeFile(
  path.join(outDir, "packages.json"),
  JSON.stringify(
    { version, packages: [...built.map((t) => t.pkg.replace(/^@[^/]+\//, "")), "cli"] },
    null,
    2,
  ) + "\n",
);
