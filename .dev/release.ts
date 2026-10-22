import { Command, spawn } from "@jpillora/take";

export default Command({
  name: "release",
  description: "Publish a tagged prerelease using GoReleaser",
  flags: {},
  async run() {
    await spawn({
      program: "goreleaser",
      args: ["release", "--clean", "--config", ".dev/goreleaser.yml"],
      stdio: "inherit",
    });
  },
});
