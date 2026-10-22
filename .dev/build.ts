import { mkdir } from "node:fs/promises";
import { Command, spawn } from "@jpillora/take";

export default Command({
  name: "build",
  description: "Build the frontend and the binary at dist/cloud-torrent",
  flags: {},
  async run({ cmd }) {
    await cmd("frontend");
    await mkdir("dist", { recursive: true });
    await spawn({
      program: "go",
      args: ["build", "-o", "dist/cloud-torrent", "."],
      stdio: "inherit",
    });
  },
});
