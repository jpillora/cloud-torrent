import { Command, spawn } from "@jpillora/take";

export default Command({
  name: "gen-types",
  description: "Regenerate frontend TypeScript types from Go structs",
  flags: {},
  async run() {
    await spawn({
      program: "go",
      args: ["tool", "tygo", "generate", "--config", ".dev/tygo.yaml"],
      stdio: "inherit",
    });
  },
});
