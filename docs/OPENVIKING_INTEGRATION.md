# OpenViking memory integration

Catena uses the official OpenViking **0.4.23** image pinned by digest. The Go control plane implements the existing `MemoryBackend` interface over its HTTP API. GauzMem remains selectable for existing installations; the OpenViking Compose overlay selects the new provider.

## Available paths

- `/memory`: browse and search memory files, add an explicit note, inspect file references and extraction tasks.
- Trace detail → **提炼为记忆**: a bounded, redacted evidence snapshot is submitted as an OpenViking session and committed for asynchronous extraction.
- Existing conversation extraction uses original message roles and chronological order, in batches of up to 100 messages.
- Platform analysis retrieves up to four relevant retained memories once, stores that snapshot as `memory_context` in its job, and supplies it as reference context to the inspector, candidate generator and reviewer. It is kept separate from current-run evidence.
- Existing memory IDs and graph routes remain compatible: a deterministic 48-bit hash identifies each canonical file URI. The URI is also returned in metadata.

## Start

Set `CATENA_LLM_API_KEY`, optionally `CATENA_LLM_BASE_URL` and `CATENA_LLM_MODEL`, then run:

```sh
python deploy/catena-mvp1/setup_openviking.py
cd deploy/catena-mvp1
docker compose --env-file ../../.local/openviking/service.env \
  -f compose.yml -f compose.openviking.yml up -d --build
```

If your existing deployment uses a separate environment file, keep passing it as well:

```sh
docker compose --env-file .env --env-file ../../.local/openviking/service.env \
  -f compose.yml -f compose.openviking.yml up -d --build
```

The generated configuration is ignored by Git. Configuration and OV data persist in `.local/openviking`; the embedding model cache persists in a Docker volume. For a Linux deployment, change the LLM URL to a reachable endpoint. The model proxy is not part of this overlay.

Neither memory nor embedding services publish host ports. Catena sends a private service key and owner-derived identity headers to OV in **trusted** mode. The browser never receives these credentials or controls those headers. The adapter restricts retrieval to the owner's memory root, rejects foreign result URIs and path escapes, and binds public task IDs to the owner.

## Local embedding

`BAAI/bge-small-zh-v1.5` runs on CPU through FastEmbed/ONNX Runtime, returning 512-dimensional vectors. FastEmbed lists an approximately 90 MB model, Chinese focus and a 512-token input limit. This is a small starting model, not a measured best model for all languages or code retrieval.

The service serializes inference, limits requests to 128 strings and 1 MiB, and responds to health checks independently. Runtime limits are 768 MiB / 1 CPU for embedding and 2 GiB / 1 CPU for OV. These are limits, not measured peak consumption.

On this workstation, downloads from the container failed with TLS errors. We downloaded the official Qdrant ONNX files through the existing host proxy into `.local/openviking/bge-small-zh`, then mounted that directory at `/prefetched` in an ignored local overlay with `CATENA_EMBEDDING_MODEL_PATH=/prefetched`. Normal deployments can use the automatic model download and persistent cache. No synthetic embedding fallback is used.

## Graph behavior

OV memory-link extraction is enabled with `memory.link_enabled=true`. Catena displays typed outgoing links from a file's native `MEMORY_FIELDS.links` metadata and explicit local Markdown links. Metadata-only relations are not assigned a fabricated confidence score. Cross-owner links are excluded. A `STORED_IN` edge represents file identity, not a semantic inference.

This adapter does not call the old standalone `relations/link` routes: they are absent from the tested 0.4.23 image. It does not yet implement a general multi-hop graph query, editing relations in the UI, or knowledge-graph compilation.

## Limits and scope

- Personal memory is implemented. Team spaces and grants are not implemented by this integration; do not equate OpenViking account support with completed Catena team sharing.
- Listing scans at most 1,000 OV nodes; `total` counts visible memory files within that scan. The existing numeric graph lookup uses the same bounded scan. File reads show at most 200 lines / 16,000 bytes.
- Trace extraction includes at most 64 spans / 64 KiB; individual evidence fields are bounded to 2 KiB. Conversation extraction has a 64 KiB text budget. Original evidence remains in Catena.
- Repeat extraction creates a new OV session; memory merging is handled by OV. There is no ingestion idempotency guarantee yet.
- Failed recall does not prevent platform analysis. No automatic bulk extraction of the historical dataset is enabled.
- GPT-5.5 calls still consume the existing proxy allowance. Local embedding removes the embedding API requirement, not extraction costs.
- Harbor Cases and Skills remain Catena's verified artifacts; this integration does not enable OV's separate Agent Evolution pipeline.

## Verification

During the local integration on 2026-10-03/04:

- Real manual-note write returned `vector_status=complete`; a Chinese query recalled the note (observed score about 0.79).
- One real Codex Trace committed successfully. OV reported **8 memory-write operations**, **37.5 seconds** processing, and **57,881 total LLM tokens**. This is one integration sample, not eight distinct facts or a quality benchmark.
- Native task polling initially failed because timestamps are numeric; mapping the provided ISO fields fixed it.
- A second explicit note linked the first and produced `FILE_LINK` plus `STORED_IN` edges.
- Owner isolation, foreign task rejection, secret redaction, stable IDs, and cross-owner graph filtering are covered by Go adapter tests.
- Go memory / evolution job tests and the server tests passed; frontend typecheck and production build passed.
- Live browser checks passed for adding a note, Chinese search, the detail reader and graph switching. These were performed against the running services, without mocked memory APIs.

Sources: [OpenViking context types](https://docs.openviking.ai/en/concepts/02-context-types), [session API](https://docs.openviking.ai/en/api/05-sessions), [FastEmbed models](https://qdrant.github.io/fastembed/examples/Supported_Models/).
