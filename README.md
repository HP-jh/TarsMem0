# TarsMem0

MCP integration for Mem0 (AI memory platform).

## Tools

- `mem0_search` — Search memories by query
- `mem0_add` — Add a new memory entry

## Quick Start

```bash
go build -o tarsmem0
TARS_MCP_ENDPOINT=https://api.example.com TARS_MCP_API_KEY=xxx ./tarsmem0
```

## Environment Variables

- `TARS_MCP_ENDPOINT` — upstream API endpoint
- `TARS_MCP_API_KEY` — upstream API key
- `TARS_MCP_MODE` — `stdio` (default) or `http`

## License

MIT
