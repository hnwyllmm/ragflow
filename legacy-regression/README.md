# Experimental actual-worker replay for #22082

Not part of the daily smoke suite. Run **alone**, in a separate Go process with a
fresh unique test email, after the full pinned `seekdb-api` Compose stack starts.
Uses the native Go API + worker + TEI + SeekDB + metadata MySQL; no mocked queue,
worker, embeddings or parsing. Baseline: `dc8768a32b7dc537e65a9cc4c9bb4dd65ed66ddc`.

Copy this trusted `legacy_worker_test.go` alongside pinned `baseline/test/api/*.go`
in the standalone `api-client` directory. No new Go module dependencies.

Example separate hosted step (do not run alongside the broad Go suite):

```bash
cp harness/legacy-regression/legacy_worker_test.go api-client/
export HOST_ADDRESS=http://127.0.0.1:9380
export RAGFLOW_TEST_EMAIL="legacy-${GITHUB_RUN_ID}-${GITHUB_RUN_ATTEMPT}@example.invalid"
export SEEKDB_LEGACY_DISPOSABLE=1
export SEEKDB_LEGACY_RUNTIME="$PWD/runtime"
export SEEKDB_LEGACY_PROJECT=seekdb-api
export SEEKDB_LEGACY_FIXTURE="$PWD/harness/legacy-regression/legacy-fixture.sh"
(cd api-client && go test -tags integration -count=1 -timeout=15m -json \
  -run '^TestSeekDBLegacyTableForcedReparseWorker22082$' .) > artifacts/legacy-worker.json 2>&1
printf '%s\n' TestSeekDBLegacyTableForcedReparseWorker22082 > artifacts/legacy-case.txt
go run harness/ci/verify-api.go artifacts/legacy-case.txt artifacts/legacy-worker.json
```

Use shell `pipefail`/exit-code propagation in the actual workflow and ensure the
verification runs even when Go exits nonzero. This case is destructive only to
its disposable tenant's chunk table, so it must not be pointed at any user data.
The SQL helper validates all IDs, verifies no other dataset/document rows exist,
and only uses the trusted Compose configuration and fixed column manifest.
It uses container environment credentials, not tokens supplied in command args.

## Assertions and boundaries

1. Upload two deterministic plain-text files and wait for both real worker runs
   to complete. Snapshot their indexed IDs and text.
2. Preserve both uploaded files and completed tasks. Remove the #22121 optional
   columns from the tenant chunk table (retaining original base columns, vector
   column and indexes). Preserve source classification in `extra` JSON.
3. Rewrite only the target's stored chunk IDs/text to a controlled obsolete
   revision. The neighbor's indexed IDs/text and uploaded source remain intact.
4. Call `POST /datasets/:id/chunks` with the target ID. This is the forced-delete
   route, not the normal `/documents/parse` no-op for completed documents.
5. Require a new completed ingestion event, restored original source text under non-legacy
   IDs, zero obsolete rows, unchanged neighbor, and vector retrieval returning
   the new worker output for the target document.
6. Verify optional columns remain absent after the worker completes.

This is a deterministic **old-index/new-source fixture**, not a test of an upload
replacement API. It intentionally avoids knowledge-compiler products or event
fanout and tests ordinary source documents. Initial full parsing is required;
fixture insertion does not stand in for the worker.

Local validation: shell syntax and standalone Go test compilation are available.
No Docker daemon/client is installed in the cloud editing environment; live SQL,
actual-worker execution and baseline/red-version behavior require hosted CI.
Pre-#22121 replay may fail at initial parsing if that version cannot write the
current source schema; such a failure is not evidence of the forced-rerun bug.
The narrower service test remains the focused pre-fix red / post-fix green test.
