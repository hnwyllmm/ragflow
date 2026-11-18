# Experimental, manual-only SeekDB API subset

Status: staged design, **not a verified hosted run**. Do not add to the daily job.
This is a separate workflow_dispatch entry, not a replacement for engine regression CI.

## Scope and provenance

Eight unmodified upstream Go API tests are selected by exact name in
`ci/api-cases.txt`. Harness and Compose/Dockerfile baseline is
`dc8768a32b7dc537e65a9cc4c9bb4dd65ed66ddc`. Blank PR uses that same
server source. A supplied upstream PR number builds that PR's actual Go server
source; the resolved SHA is saved in artifacts. PR workflows/Compose files are
never executed. Build scripts and application code are still untrusted executable
input: dispatch only a PR whose code you trust. No secrets, persisted git credentials,
SSH agents, Docker socket mounts, write permissions, or shared caches are supplied.

The upstream test files are copied unchanged into a standalone Go module because
they use only the standard library and call the server over HTTP. This avoids
building/linking server native libraries into the test **client**. The actual server
still uses the native upstream Docker build (`build.sh --cpp`, `build.sh --go`),
not CGO-disabled server stubs. This differs from upstream's general-purpose
`build.sh --test-integration-go` wrapper and is intentionally restricted to
`test/api`, never internal engine or parser tests.

Exact test launch after stack startup:

```sh
selection="^($(paste -sd'|' harness/ci/api-cases.txt))$"
cd api-client
HOST_ADDRESS=http://127.0.0.1:9380 go test -tags integration -count=1 \
  -timeout 25m -json -run "$selection" .
```

The verifier requires RUN and PASS for all eight cases and rejects all skipped
subtests, not merely exit code zero or a green job with zero tests. The test
selection excludes paid-provider chat, keyword expansion and reranking cases;
these are **not covered**, rather than silently counted as passing.

## Services and fixtures

- Real SeekDB pinned to the same 1.4.0 digest as the existing smoke job.
- Native Go RAGFlow API/admin/ingestor services built from candidate source.
- MySQL metadata, MinIO-compatible object store, Kvrocks, NATS, ClickHouse through
  the pinned upstream Compose definition.
- CPU TEI, public `BAAI/bge-small-en-v1.5`. No chat or reranker credentials.
  Upstream harness self-registers a local disposable tenant and sets the Builtin
  embedding default; tests fail if real embedding/indexing is unusable.
- Actual upstream fixture helpers create isolated KBs/documents/chunks and clean
  them up. No external account or preloaded user dataset.

The selected retrieval fixture defaults `vector_similarity_weight=0`: it covers
retrieval endpoints and isolation, **not** a dense-vector ranking quality benchmark.
The separate filtered dense top-k engine regression must remain.

## Resource and validation gates

The standard Ubuntu hosted runner is the target, but viability is unproven.
The upstream builder and deps images contain large native/model resources and
normally run on cached self-hosted infrastructure. Frontend build was removed
from this API-only image; no other server implementation was reduced. Initial
uncached downloads, peak disk, peak RAM and time must be measured on the first
manual run. Workflow timeout 90 minutes and API timeout 25 minutes are bounds,
not measured duration estimates. Logs and before/after disk snapshots are
uploaded on failure. Do not schedule daily or claim default-suite equivalence
until the first run passes and resource measurements are reviewed.

Unpinned ancillary upstream image tags are inherited from the immutable Compose
text, not immutable container content. Record/pin their digests after the first
validated pull before promoting this experiment to routine CI. Native dependency
fallback uses official upstream download scripts without Gitee credentials.
If those downloads require unavailable artifacts, report that as a build blocker;
do not substitute a stale prebuilt server and claim PR coverage.

No Python SDK suite, chat/agent completion, PDF/OCR fidelity, all-backend matrix,
or exhaustive upstream suite is covered by this entry.

`TestRESTDocumentParsingValidationCompletionAndRepeatedStart` uses the upstream
API contract and repeats an ordinary parse start. It does **not** establish the
historical-table + worker `RerunWithDelete` path of issue #22082; keep the explicit
legacy-table reparsing regression in its separate job.
