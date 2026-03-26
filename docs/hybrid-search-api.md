# Hybrid Search API

## `POST /v1/spaces/{space_id}/search`

Hybrid search combining full-text search (Tantivy) with semantic vector search (Qdrant). Results are ranked using Reciprocal Rank Fusion (RRF) where vector results dominate (0.7 weight) over FTS (0.3 weight).

## How it works

1. **FTS path**: Searches object names and snippets using text matching (existing behavior)
2. **Vector path**: Embeds the query via Together AI (`intfloat/multilingual-e5-large-instruct`), searches Qdrant for semantically similar chunks across all page-like objects (basic, todo, note, bookmark layouts)
3. **RRF reranking**: Merges both result sets by rank position, not raw scores. Vector-only results (semantic matches with no keyword overlap) are included
4. **Deduplication**: Multiple chunk matches within the same object are collapsed — only the best-matching chunk is returned per object

## Request

```json
POST /v1/spaces/{space_id}/search
Content-Type: application/json

{
  "query": "cognitive behavioral therapy emotions",
  "types": ["page"],
  "sort": {
    "property_key": "last_modified_date",
    "direction": "desc"
  },
  "filters": null
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `query` | string | no | Search text. When provided, enables hybrid search. When empty, returns all objects matching filters/types |
| `types` | string[] | no | Filter by type keys (e.g. `["page", "note", "task"]`) |
| `sort` | object | no | Sort options (applied to FTS results before merging) |
| `filters` | object | no | Advanced nested AND/OR filter expressions |

Query params: `?offset=0&limit=100` (pagination, max 1000)

## Response

```json
{
  "data": [
    {
      "object": "object",
      "id": "bafyreibkiuamsedqfoqrodxfkm746v2vzsriboomqj2hjvztlxusi4xohy",
      "name": "Grandmas Sourdough Bread Recipe",
      "icon": { "format": "emoji", "emoji": "🍞" },
      "archived": false,
      "space_id": "bafyreib...29p0x9y2bhkp4",
      "snippet": "Making sourdough bread requires patience...",
      "layout": "basic",
      "type": { "key": "page", "name": "Page" },
      "properties": [],
      "matched_chunk": {
        "title": "",
        "text": "Grandmas Sourdough Bread Recipe\nMaking sourdough bread requires patience and a healthy starter culture...",
        "score": 0.91
      }
    },
    {
      "object": "object",
      "id": "bafyreid44dhmgt7kao76hlhxrdgxw6sjiwsd2qxhxtnhon5b35urha6noq",
      "name": "Quantum Computing Basics",
      "matched_chunk": null
    }
  ],
  "pagination": {
    "total": 5,
    "offset": 0,
    "limit": 100,
    "has_more": false
  }
}
```

## The `matched_chunk` field

Present on results that were found via vector search. `null` for FTS-only results.

| Field | Type | Description |
|-------|------|-------------|
| `title` | string | Section header of the matching chunk (empty if preamble text before any header) |
| `text` | string | The actual chunk text that matched semantically (up to ~1000 chars) |
| `score` | float | Cosine similarity score from vector search. Useful for relative comparison within a result set, not as an absolute threshold |

## Behavior notes

- **No query**: When `query` is empty, behaves as pure FTS/filter search (no vector search). `matched_chunk` will be `null` on all results
- **Vector search disabled**: When `TOGETHER_API_KEY` is not set or `ANYTYPE_VECTOR_SEARCH_ENABLED=false`, falls back to FTS-only. Fully backward compatible
- **Indexed layouts**: Only `basic`, `todo`, `note`, and `bookmark` layouts are indexed for vector search. Other object types (relations, types, tags, participants, files) are excluded
- **Score interpretation**: The E5 model produces scores in a compressed 0.7–1.0 range. Only relative ordering matters. Results are filtered to within 97% of the top score to remove noise
- **Chunking**: Documents are split by headers (H1–H3) into chunks of ≤1000 chars. Large sections are split into continuation pieces. Each chunk is embedded independently

## Global search variant

```
POST /v1/search
```

Same request/response format but searches across all spaces. Vector search is applied per-space then results are merged.

## Configuration (env vars)

| Variable | Default | Description |
|----------|---------|-------------|
| `TOGETHER_API_KEY` | — | Together AI API key for embeddings |
| `ANYTYPE_VECTOR_SEARCH_ENABLED` | `true` | Enable/disable vector search |
| `ANYTYPE_QDRANT_ADDR` | `http://localhost:6333` | Qdrant server address |
| `ANYTYPE_EMBEDDING_API_URL` | `https://api.together.xyz/v1/embeddings` | Embeddings API endpoint (OpenAI-compatible) |
| `ANYTYPE_EMBEDDING_MODEL` | `intfloat/multilingual-e5-large-instruct` | Embedding model name |
| `ANYTYPE_EMBEDDING_DIMENSIONS` | `1024` | Vector dimensions (must match Qdrant collection) |
| `ANYTYPE_VECTOR_SEARCH_SKIP_TYPES` | — | Comma-separated type keys to exclude from indexing (e.g. `ot-program,ot-debugLog`) |
