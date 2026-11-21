// Maps the running platform to the npm package that carries its binary.
//
// npm and Go disagree on names — win32 vs windows, x64 vs amd64 — and getting
// the mapping wrong means npm installs nothing for that platform and the CLI
// fails at run time with no useful message. The table is therefore explicit
// rather than computed, and a test pins it against the CI build matrix.

export const SCOPE = "@alvaroofernaandez";
export const CLI_PACKAGE = `${SCOPE}/aval`;

/**
 * Every platform we ship. `platform`/`arch` are Node's names (process.platform,
 * process.arch); `goos`/`goarch` are Go's.
 */
export const TARGETS = [
  { platform: "darwin", arch: "arm64", goos: "darwin", goarch: "arm64" },
  { platform: "darwin", arch: "x64", goos: "darwin", goarch: "amd64" },
  { platform: "linux", arch: "arm64", goos: "linux", goarch: "arm64" },
  { platform: "linux", arch: "x64", goos: "linux", goarch: "amd64" },
  { platform: "win32", arch: "arm64", goos: "windows", goarch: "arm64" },
  { platform: "win32", arch: "x64", goos: "windows", goarch: "amd64" },
].map((t) => ({ ...t, pkg: `${CLI_PACKAGE}-${t.platform}-${t.arch}` }));

/** The package holding the binary for this platform, or null if unsupported. */
export function packageFor(platform, arch) {
  const t = TARGETS.find((t) => t.platform === platform && t.arch === arch);
  return t ? t.pkg : null;
}

/** The binary's filename on this platform. */
export function binaryName(platform) {
  return platform === "win32" ? "aval.exe" : "aval";
}

/**
 * The message shown on an unsupported platform. It names the platform and
 * points at building from source, which needs nothing but Go — being on an
 * odd architecture should not mean being stuck.
 */
export function describeUnsupported(platform, arch) {
  const supported = TARGETS.map((t) => `${t.platform}/${t.arch}`).join(", ");
  return [
    `aval has no prebuilt binary for ${platform}/${arch}.`,
    ``,
    `Supported: ${supported}`,
    ``,
    `Install from source instead:`,
    `  go install github.com/alvaroofernaandez/aval/cmd/aval@latest`,
  ].join("\n");
}
