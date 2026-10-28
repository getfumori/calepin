import { readdir, rm } from "node:fs/promises";

const dist = new URL("../internal/webassets/dist/", import.meta.url);
for (const entry of await readdir(dist)) {
  if (entry !== ".keep") {
    await rm(new URL(entry, dist), { recursive: true, force: true });
  }
}
