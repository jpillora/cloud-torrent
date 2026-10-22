import { Command, spawn } from "@jpillora/take";

export default Command({
  name: "e2e",
  description:
    "Verify UI, realtime sync and downloaded file/ZIP contents in Chromium",
  flags: {
    skipBuild: {
      initial: false,
      description: "Use the existing dist/cloud-torrent binary",
    },
    installBrowser: {
      initial: false,
      description: "Install the pinned Playwright Chromium",
    },
    withDeps: {
      initial: false,
      description: "Install Chromium OS dependencies too (may require sudo)",
    },
    publicFixture: {
      initial: false,
      description: "Use the public Big Buck Bunny poster (requires internet)",
    },
  },
  async run({ cmd, flags }) {
    if (!flags.skipBuild) await cmd("build");
    await spawn({
      program: "npm",
      args: ["--prefix", ".dev/e2e", "ci"],
      stdio: "inherit",
    });
    if (flags.installBrowser || flags.withDeps) {
      await spawn({
        program: "npm",
        args: [
          "--prefix",
          ".dev/e2e",
          "exec",
          "--",
          "playwright",
          "install",
          ...(flags.withDeps ? ["--with-deps"] : []),
          "chromium",
        ],
        stdio: "inherit",
      });
    }
    await spawn({
      program: process.execPath,
      args: [
        ".dev/e2e/core.ts",
        ...(flags.publicFixture ? ["--public-fixture"] : []),
      ],
      stdio: "inherit",
    });
  },
});
