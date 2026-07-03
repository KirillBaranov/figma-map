#!/usr/bin/env node

import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { Election } from "./election.js";
import { Node } from "./node.js";
import { registerTools } from "./tools.js";
import { VERSION } from "./version.js";

const LISTEN_PORT = 1994;

async function bootstrap(): Promise<void> {
  const node = new Node(LISTEN_PORT);
  const election = new Election(LISTEN_PORT, node);
  await election.start();

  const shutdown = (): void => {
    console.error("Shutting down...");
    election.stop();
    node.stop();
    process.exit(0);
  };
  process.once("SIGINT", shutdown);
  process.once("SIGTERM", shutdown);

  const mcpServer = new McpServer({
    name: "figma-bridge",
    version: VERSION,
  });
  registerTools(mcpServer, node, LISTEN_PORT);

  console.error(`Starting MCP server (role: ${node.roleName})`);
  await mcpServer.connect(new StdioServerTransport());
}

bootstrap().catch((err) => {
  console.error("Fatal error:", err);
  process.exit(1);
});
