import { Command, spawn } from "@jpillora/take";

export default Command({
  name: "test",
  description: "Run the Go test suite",
  flags: {},
  async run() {
    await spawn({
      program: "go",
      args: ["test", "-v", "./..."],
      stdio: "inherit",
    });
  },
});
