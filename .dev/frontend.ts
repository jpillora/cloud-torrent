import { Command, spawn } from "@jpillora/take";

export default Command({
  name: "frontend",
  description: "Install frontend dependencies and build the embedded SPA",
  flags: {},
  async run() {
    await spawn({
      program: "npm",
      args: ["--prefix", "frontend", "ci"],
      stdio: "inherit",
    });
    await spawn({
      program: "npm",
      args: ["--prefix", "frontend", "run", "build"],
      stdio: "inherit",
    });
  },
});
