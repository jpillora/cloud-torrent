import { createHash } from "node:crypto";
import { createServer } from "node:http";
import { setTimeout as delay } from "node:timers/promises";

// Authored deterministic CC0 test content. No external trackers or data sources.
export const name = "Fixture # +% ü";
export const files = new Map([
  ["first # +%.txt", Buffer.from("Cloud Torrent CC0 fixture A\n".repeat(8192))],
  [
    "nested/second ü.txt",
    Buffer.from("Cloud Torrent CC0 fixture B\n".repeat(8192)),
  ],
]);

type Value = string | number | Buffer | Value[] | { [key: string]: Value };
function encode(value: Value): Buffer {
  if (typeof value === "number") return Buffer.from(`i${value}e`);
  if (typeof value === "string") value = Buffer.from(value);
  if (Buffer.isBuffer(value)) {
    return Buffer.concat([Buffer.from(`${value.length}:`), value]);
  }
  if (Array.isArray(value)) {
    return Buffer.concat([
      Buffer.from("l"),
      ...value.map(encode),
      Buffer.from("e"),
    ]);
  }
  return Buffer.concat([
    Buffer.from("d"),
    ...Object.keys(value).sort().flatMap((
      key,
    ) => [encode(key), encode(value[key])]),
    Buffer.from("e"),
  ]);
}

const data = Buffer.concat([...files.values()]);
const pieceLength = 16 * 1024;
const pieces: Buffer[] = [];
for (let offset = 0; offset < data.length; offset += pieceLength) {
  pieces.push(
    createHash("sha1").update(data.subarray(offset, offset + pieceLength))
      .digest(),
  );
}
const info = {
  name,
  "piece length": pieceLength,
  pieces: Buffer.concat(pieces),
  files: [...files].map(([path, bytes]) => ({
    length: bytes.length,
    path: path.split("/"),
  })),
};
export const infohash = createHash("sha1").update(encode(info)).digest("hex");

export async function startFixture() {
  let torrent: Buffer;
  const server = createServer(async (req, res) => {
    if (req.url === "/fixture.torrent") {
      res.writeHead(200, { "Content-Type": "application/x-bittorrent" });
      res.end(torrent);
      return;
    }
    const path = decodeURIComponent(
      new URL(req.url!, "http://localhost").pathname,
    );
    const content = files.get(path.slice(`/${name}/`.length));
    if (!path.startsWith(`/${name}/`) || !content) {
      res.writeHead(404).end();
      return;
    }
    const range = /^bytes=(\d+)-(\d*)$/.exec(req.headers.range ?? "");
    const start = range ? Number(range[1]) : 0;
    const end = range?.[2]
      ? Math.min(Number(range[2]), content.length - 1)
      : content.length - 1;
    if (start > end) {
      res.writeHead(416).end();
      return;
    }
    res.writeHead(range ? 206 : 200, {
      "Content-Type": "application/octet-stream",
      "Content-Length": end - start + 1,
      "Accept-Ranges": "bytes",
      ...(range
        ? { "Content-Range": `bytes ${start}-${end}/${content.length}` }
        : {}),
    });
    for (let offset = start; offset <= end && !res.destroyed; offset += 8192) {
      await delay(70);
      if (!res.destroyed) {
        res.write(content.subarray(offset, Math.min(offset + 8192, end + 1)));
      }
    }
    res.end();
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  if (!address || typeof address === "string") {
    throw new Error("fixture listener missing");
  }
  const url = `http://127.0.0.1:${address.port}`;
  torrent = encode({ info, "url-list": [url + "/"] });
  return {
    magnet: `magnet:?xt=urn:btih:${infohash}&dn=${
      encodeURIComponent(name)
    }&xs=${encodeURIComponent(url + "/fixture.torrent")}`,
    close: async () => {
      server.closeAllConnections();
      await new Promise<void>((resolve, reject) =>
        server.close((err) => err ? reject(err) : resolve())
      );
    },
  };
}
