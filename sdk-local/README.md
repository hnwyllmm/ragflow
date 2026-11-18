# Caller-provisioned, local-model SDK experiment

This is a **separate** result from the unchanged upstream SDK auto-provisioning
attempt. Do not replace or relabel that attempt's engine/credential failure.
No test assertions, upstream fixture code or server code are changed.

Run from the same workspace after the real server/SeekDB/TEI stack and SDK
Python environment are ready:

```
bash harness/sdk-local/run.sh dataset
bash harness/sdk-local/run.sh document chunk
# or all three, serially (up to 45 minutes plus bootstrap):
bash harness/sdk-local/run.sh all
```

The default single dataset group is intentionally the smallest first trial.
Each group has a 15-minute test limit and a 3-minute provisioning limit. Every
group gets a fresh owner tenant and unrelated secondary tenant. Never reuse the
Go test tenant or another folder's tenant. Data cleanup remains the original
fixtures' responsibility; the whole disposable Compose volume teardown removes
leftover tenants/data after failure. There are no user data or shared tenants.

`bootstrap.go` performs actual register/login/token/default-embedding HTTP calls
on the hard-limited `http://127.0.0.1:9380` address. It reads the existing public
upstream encrypted test-password fixture from pinned configs.py. Only the
Builtin Local BAAI/bge-small-en-v1.5 embedding is selected. It neither registers
paid providers nor lies about a chat/rerank model. Tokens are stored in a private
0600 temporary file, not artifacts; env export uses jq values, never shell eval.

We set **RAGFLOW_SDK_CI=0**, the existing upstream hook's explicit opt-out from
its automatic provisioning, then supply the caller-owned environment that the
original folder fixtures already accept. `RAGFLOW_SDK_ISOLATED=1` accurately
reflects freshly created tenants. DOC_ENGINE stays seekdb. Empty external-key
variables permit configs.py import, not authentication to a provider.

## Exact pinned p2 selection

Baseline: dc8768a32b7dc537e65a9cc4c9bb4dd65ed66ddc.
Collection with original root pyproject and `--level=p2` discovered:

| Folder | integration cases | service-free contract cases |
|---|---:|---:|
| test_dataset_mangement | 46 | 27 |
| test_file_management_within_dataset | 98 | 11 |
| test_chunk_management_within_dataset | 11 | 11 |
| Total | 155 | 49 |

The 155 are **discovered selected cases, not confirmed compatible passes**.
No integration has been executed by this harness locally yet. `cases-dc8768a.json`
is an exact static collection manifest (JUnit class/name to integration marker).
Each folder's JUnit is compared against it: pass, failure, error, skip and
not-executed are separate. Missing/unexpected cases and skips fail the experiment
instead of inflating pass counts. Root pytest p2 filters are unchanged.

These p2 tests cover primarily listing/deletion/upload and selected chunk
validation; many richer create/update/retrieval cases are unmarked and therefore
not selected by upstream p2. No claim of exhaustive SDK/database parity.

## Evidence

`artifacts/sdk-local/<group>/` contains bootstrap status (redacted on error),
pytest log and exit code, JUnit, and count summary split into integration/contract.
A failed group does not suppress later selected groups. Nonzero final exit means
at least one setup, test, coverage or skip gate failed. The host job must preserve
these artifacts independently from broad Go and original SDK results.

Prerequisites: working real TEI embedding/model bootstrap, native Go server,
SeekDB, original SDK Python 3.13 dependencies/plugins, Go, jq. No pipeline or
chat model is provisioned because this exact p2 selection does not require it.
If the manifest or selection expands, re-evaluate requirements rather than
manufacture model IDs or weaken original assertions.
