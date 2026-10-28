import { readFileSync, writeFileSync } from "node:fs";

import { expect, test } from "vitest";
import { z } from "zod";

import * as api from "../../src/web/contracts/api.js";
import * as connections from "../../src/web/contracts/connections.js";
import * as daily from "../../src/web/contracts/daily-note.js";
import * as notes from "../../src/web/contracts/human-note.js";
import * as model from "../../src/web/contracts/organization-model.js";
import * as search from "../../src/web/contracts/search.js";
import * as tasks from "../../src/web/contracts/tasks.js";
import * as config from "../../src/web/contracts/app-config.js";

const { endpoints, errorResponseSchema } = api;

const documentPath = new URL("../../api/openapi.json", import.meta.url);

// Every exported schema is a named component, so each type is defined once
// and endpoints refer to it.
const registry = z.registry<{ id: string }>();
const names = new Map<z.ZodType, string>();
for (const module of [api, connections, daily, notes, model, search, tasks, config]) {
  for (const [name, value] of Object.entries(module)) {
    if (value instanceof z.ZodType && name.endsWith("Schema") && !names.has(value)) {
      const id = name.slice(0, -"Schema".length).replace(/^./, (c) => c.toUpperCase());
      names.set(value, id);
      registry.add(value, { id });
    }
  }
}

function schema(type: z.ZodType): unknown {
  const id = names.get(type);
  if (!id) throw new Error("endpoint schemas must be exported from src/web/contracts");
  return { $ref: `#/components/schemas/${id}` };
}

function components(): unknown {
  const generated = z.toJSONSchema(registry, {
    io: "input",
    unrepresentable: "any",
    cycles: "ref",
    uri: (id) => `#/components/schemas/${id}`
  });
  const out: Record<string, unknown> = {};
  for (const [id, value] of Object.entries(generated.schemas)) {
    const { $schema: _ignored, ...rest } = value as Record<string, unknown>;
    out[id] = rest;
  }
  return out;
}

function operationId(endpoint: api.Endpoint): string {
  const words = endpoint.path
    .replace("/api/v1/", "")
    .split(/[/-]/)
    .map((part) => (part.startsWith("{") ? `By${part.slice(1, -1)}` : part))
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1));
  return endpoint.method + words.join("");
}

function buildDocument(): unknown {
  const paths: Record<string, Record<string, unknown>> = {};
  for (const endpoint of endpoints) {
    const parameters = [
      ...[...endpoint.path.matchAll(/\{(\w+)\}/g)].map((match) => ({
        name: match[1],
        in: "path",
        required: true,
        schema: { type: "string" }
      })),
      ...Object.entries(endpoint.query ?? {}).map(([name, description]) => ({
        name,
        in: "query",
        required: name === "q" && endpoint.path.endsWith("/search"),
        description,
        schema: { type: "string" }
      })),
      ...(endpoint.idempotent
        ? [{ name: "Idempotency-Key", in: "header", required: false, description: "Replays the recorded response when a request is retried", schema: { type: "string" } }]
        : [])
    ];
    const status = String(endpoint.status ?? 200);
    paths[endpoint.path] ??= {};
    paths[endpoint.path]![endpoint.method] = {
      operationId: operationId(endpoint),
      summary: endpoint.summary,
      ...(parameters.length > 0 ? { parameters } : {}),
      ...(endpoint.request
        ? { requestBody: { required: true, content: { "application/json": { schema: schema(endpoint.request) } } } }
        : {}),
      responses: {
        [status]: endpoint.response
          ? { description: "Success", content: { "application/json": { schema: schema(endpoint.response) } } }
          : { description: "Success" },
        default: {
          description: "Error: not_found, stale_revision, conflict, invalid_canonical_markdown, invalid_query, or bad_request",
          content: { "application/json": { schema: schema(errorResponseSchema) } }
        }
      }
    };
  }
  return {
    openapi: "3.1.0",
    info: {
      title: "Fumori Vault Service",
      version: "1",
      description:
        "Generated from src/web/contracts/api.ts by `pnpm run openapi`. Do not edit by hand. The service has no authentication of its own; it listens on loopback or behind an operator-controlled authenticating gateway, so no security scheme is declared."
    },
    servers: [{ url: "http://127.0.0.1:3000", description: "Default local listener" }],
    paths,
    components: { schemas: components() }
  };
}

test("api/openapi.json matches the contracts", () => {
  const generated = `${JSON.stringify(buildDocument(), null, 2)}\n`;
  if (process.env.FUMORI_WRITE_OPENAPI === "1") {
    writeFileSync(documentPath, generated);
  }
  const committed = readFileSync(documentPath, "utf8");
  expect(committed, "api/openapi.json is stale; run `pnpm run openapi`").toBe(generated);
});
