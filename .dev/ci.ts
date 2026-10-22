import { Command } from "@jpillora/take";

export default Command({
  name: "ci",
  description: "Check generated types, build, and run tests",
  flags: {},
  async run({ cmd }) {
    await cmd("check-types");
    await cmd("build");
    await cmd("test");
  },
});
