import { Command, spawn } from "@jpillora/take";

export default Command({
  name: "check-types",
  description: "Regenerate types and fail if committed types are stale",
  flags: {},
  async run({ cmd }) {
    await cmd("gen-types");
    await spawn({
      program: "git",
      args: ["diff", "--exit-code", "--", "frontend/src/types"],
      stdio: "inherit",
    });
  },
});
