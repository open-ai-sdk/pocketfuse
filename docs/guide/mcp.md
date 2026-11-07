# MCP for AI agents

Pocketfuse serves the [Model Context Protocol](https://modelcontextprotocol.io) at
`/mcp`, so an AI agent can debug from your traces. All tools are read-only.

## Connect

The endpoint runs in the server process, at its own address. There is nothing else
to start. Replace the port if you changed `POCKETFUSE_PORT`.

Claude Code:

```sh
claude mcp add --transport http pocketfuse http://127.0.0.1:7625/mcp
```

Clients configured with JSON, such as Claude Code's `.mcp.json` and Cursor:

```json
{
  "mcpServers": {
    "pocketfuse": { "type": "http", "url": "http://127.0.0.1:7625/mcp" }
  }
}
```

Codex (`~/.codex/config.toml`):

```toml
[mcp_servers.pocketfuse]
url = "http://127.0.0.1:7625/mcp"
```

A client that only speaks stdio can bridge with
`npx mcp-remote http://127.0.0.1:7625/mcp --allow-http`.

## Tools

| Tool | Use it to |
|---|---|
| `list_sessions` | Find a conversation by its opening message and failed spans. |
| `list_traces` | Search by name or id prefix, session or thread, user, project, status (`error`, `pending`, `finished`), `errors_only` and time. |
| `get_session_timeline` | Read a conversation in order: each turn's message, reply, model calls, tool calls and errors. |
| `get_trace` | Read one trace as a compact tree. `raw_spans` shows nested spans as recorded. |
| `get_observation` | Read one span's input, output and metadata in chunks. |
| `recent_errors` | List failed spans, from the last 24 hours by default. |
| `slow_calls` | Get p50, p95, max and total by kind and name, plus the slowest spans. |

Times accept RFC 3339 or an age such as `90m`, `24h` or `7d`.

## Bounds

No response exceeds 32 KB, so one call cannot flood a model's context:

- Lists default to 10 to 80 rows depending on the tool, with a ceiling for each.
  Rows also stop at a size budget.
- A cut list says so in `truncated` and gives `nextOffset`.
- A clipped message carries `totalChars`, and other clipped text ends in `[+N chars]`.
- `get_observation` returns 4,000 characters per field by default, up to 20,000 with
  `field`. `offset` continues from `nextOffset`, and `from_end` reads the end of a
  long prompt, where the latest messages are.
- `slow_calls` measures the newest 20,000 spans in the window and says so if it
  stopped there.

## Credentials

Before any text leaves a tool, Pocketfuse replaces what it recognizes as a credential
with a `[REDACTED:kind]` marker, and the response's `redactions` field counts them.
This covers fields such as `authorization`, `cookie`, `x-api-key`, `password` and
`*_token` with string values, `Bearer` and `Basic` values, JWTs, common provider key
formats, and credentials in URL query strings. Offsets count the redacted text.
Recognizing secrets is best-effort: avoid sending them in spans at all. The web UI and
JSON API show stored data unredacted.

Other personal data in spans, such as a `user_email` attribute, is returned as stored.

## Transport and security

The endpoint is Streamable HTTP, stateless, answering with JSON. It serves protocol
revision `2026-07-28` and earlier clients using the `initialize` handshake.

MCP has no authentication of its own here: it binds where the server binds, and has
the access the rest of the server's API has. Keep it on loopback or a trusted network.
A request carrying an `Origin` header is refused unless the origin is loopback, and
requests to a loopback address must also name a loopback `Host`, which stops web pages
reaching it through DNS rebinding. Tools that are not browsers send no `Origin`, so they
work from any address the server listens on.
