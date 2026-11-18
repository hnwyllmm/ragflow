# Experimental, manual-only SeekDB API subset

Status: the strict eight-case mode passed hosted run 44259994978. The newly staged
broad mode is **not yet verified**. Do not add it to the daily job.
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

The strict subset has run on standard Ubuntu hosted infrastructure; the broad
mode resource and runtime envelope remains unverified.
The upstream builder and deps images contain large native/model resources and
normally run on cached self-hosted infrastructure. Frontend build was removed
from this API-only image; no other server implementation was reduced. Initial
uncached downloads, peak disk, peak RAM and time must be measured on the first
manual run. Workflow timeout 90 minutes and API timeout 25 minutes are bounds,
not measured duration estimates. Logs and before/after disk snapshots are
uploaded on failure. Do not schedule daily or claim default-suite equivalence
until broad-mode execution and resource measurements are reviewed.

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

## Broad mode (manual experiment)

`mode=strict` preserves the eight-case no-skip gate. `mode=broad` runs **all**
unchanged Go `test/api` tests from the pinned baseline, without a `-run` filter,
against the same native candidate server and real SeekDB. Go JSON, raw exit code,
and a Markdown report distinguish top-level pass/fail/skip/unfinished from nested
subtest totals. A successful invocation with skips is not called full coverage.

After Go finishes, a separate step installs only the original Python SDK and its
SDK test dependency group, then attempts upstream SDK p2 collection and execution.
It uses Python 3.13; it does not install the legacy Python server. SDK failure is
captured independently, so Go failures do not suppress its evidence (nor vice
versa). The final job remains failed if either attempted suite fails. Cancellation,
stack/build failure, or total job timeout can still prevent later execution.

Known SDK blockers at this baseline:
- `test/testcases/configs.py` requires the two model-key variables to exist at
  import. This runner supplies **empty strings**, not fake credentials.
- Original `_prepare_ci_environment` requires DOC_ENGINE infinity/elasticsearch
  and nonempty ZHIPU/SILICONFLOW credentials. We keep DOC_ENGINE=seekdb truthfully.
- Therefore integration provisioning is expected to stop the original SDK run.
  Collection counts and any service-free SDK contract tests are separate evidence,
  not evidence that SDK database integration passed. A local-only SDK integration
  adapter would be a distinct future change, not an unchanged upstream result.

Broad mode has a 45-minute Go limit, 15-minute SDK execution limit, 10-minute SDK
setup limit and a 120-minute job cap. Strict mode retains its original 90-minute
job cap. The full native server build has already passed in run 44259994978;
this broad mode itself has not yet been hosted/run. Chat, paid reranking and other
model-dependent tests may skip or fail and are reported, not weakened or excluded.

Local SDK feasibility check (copied upstream tests, Python 3.13): installation
succeeded, p2 collection selected 339 of 1535 cases (1196 deselected), and the
unchanged SDK run exited 1 at the supported-DOC_ENGINE provisioning guard before
any integration ran. This is collection/provisioning evidence only, not a SeekDB
SDK pass. Hosted artifacts should reproduce the exact blocker.
