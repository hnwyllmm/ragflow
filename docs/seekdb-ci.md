# Personal SeekDB CI

This is a deliberately small, independent integration check, not RAGFlow's complete
API, Python, ingestion, or end-to-end test suite. It exercises the Go OceanBase /
SeekDB storage engine against real, disposable SeekDB and OceanBase CE servers. The trusted
`TestLegacyStorageRoundTrip` asserts chunk storage, JSON/metadata compatibility,
and vector-search round trips. A skipped or missing test is a failure.

## Installation

1. Put `.github/workflows/seekdb-ci.yml` on the personal fork's `seekdb-ci` branch.
2. Put `.github/workflows/seekdb-daily.yml` on the fork's **default branch**.
   Scheduled workflows only run from the default branch. This small entrypoint
   invokes the implementation on `seekdb-ci`; its manual trigger also accepts a
   PR number. Do not copy the daily entrypoint onto a non-default branch expecting
   its cron to run there.
3. Enable these workflows on the fork, keeping inherited upstream workflows
   disabled. The fork owner may need to explicitly enable Actions in a fork.
4. Run **Personal SeekDB CI → Run workflow**. Leave `pr` blank to test upstream
   `main`, or enter a numeric PR number from `infiniflow/ragflow`.

Daily run: **06:23 UTC+08:00** (22:23 UTC). GitHub schedules are best effort and
may be delayed. GitHub can disable schedules after 60 days of inactivity in a
public repository. Check the Actions page if daily runs stop.

## What is tested

The workflow anonymously fetches `refs/heads/main` or `refs/pull/N/head`, then
checks out the fetched immutable commit. The results record the exact source
SHA, trusted test SHA and image digest. A PR head is tested **as-is**, not merged
with current main. Input cannot select another repository or shell expression.

The integration test is downloaded from the reviewed upstream commit
`dc8768a32b7dc537e65a9cc4c9bb4dd65ed66ddc` and checksum-verified, replacing that one
file in the working tree. Production engine code comes from the selected source
SHA. The smoke test contract thus stays fixed even when a PR edits or removes
its own tests. Incompatible API changes fail compilation rather than silently
weakening assertions. Other package test files and package initialization still
come from the selected PR, so this is not a security sandbox for malicious code.

The image is official `oceanbase/seekdb:1.4.0-100000172026082615`, pinned to
`sha256:4db4961869d554313520d6081fca29265ba6da6db7313311bd8acbed771a956b`.
It is not `latest`. Upgrade the tag and digest together after validating a new
version. Both databases listen only on runner loopback with Docker-allocated host ports and per-run container names; its password is a disposable
public test value, not a repository secret. SQL readiness must succeed before
Go tests run. JSON test events must show both the required test and package
passed, with no skips. `-count=1` disables cached test success.

## Safety and cost

Only standard `ubuntu-24.04` GitHub-hosted runners are used. OceanBase CE needs the public-repository runner memory allowance (16 GB); its container has an 8 GB cap. There are no upstream
status updates, PR comments, deployments, `pull_request_target`, repository write
permissions, secret inheritance, persisted Git credentials, shared caches, or
self-hosted runners. Actions are SHA-pinned. Logs are retained for seven days.
Database jobs are limited to 40 minutes and the migration job to 15 minutes. Failed runs stay failed and save diagnostics.

PR code necessarily executes on the runner, with normal runner network and Docker
access. Only request PRs you intend to execute. Do not add secrets, privileged
cloud credentials, persistent runners, or production database access to this job.
The private fork's Actions billing/quotas, if applicable, remain the owner's
responsibility. This does not purchase larger runners or enable paid services.

## Evidence and troubleshooting

The `storage-<backend>-<run>-<attempt>` artifact holds source provenance, SQL readiness,
database version, JSON test events, and container logs. The `migration-<run>-<attempt>` artifact holds the source SHA and pytest JUnit report. The run summary includes
the exact tested SHA. Missing Go toolchain, unavailable packages, compilation
errors, image pull errors, database readiness failure, assertions, and timeouts
are real failures, never converted to success.

Implementation validation in this workspace covers YAML structure, shell
syntax, source checksum, and positive/negative JSON-result contracts. Docker is
not installed here; the live container integration must be verified by the first
hosted run. Static checks do not demonstrate a passing real-database test.

## Origin and migration from upstream PR #21036

Based on the coverage proposed in https://github.com/infiniflow/ragflow/pull/21036
(head `7d5cd9432c67fddc80b8788911254f0b55d3d3f6`). That PR changed only the
workflow, not application tests. This workflow preserves its three selections:

- Python migration suite: `uv run --python 3.13 --extra dev pytest -q` from
  `tools/es-to-oceanbase-migration`, plus JUnit evidence and a no-skip check.
- `TestLegacyStorageRoundTrip` on OceanBase CE 4.4.1.0.
- `TestLegacyStorageRoundTrip` on SeekDB (now version/digest pinned).

All jobs use the single immutable upstream SHA produced by the resolver.
OceanBase CE is pinned to image tag `4.4.1.0-100000032025101610` and digest
`sha256:bb60d41b8731744bb351eb01395822efff07b5750b4745bdb0e44dd804a40963`.
Its official mini-mode bootstrap creates a `ragflow` tenant; readiness verifies
the actual tenant password and creates the `ragflow_doc` database if needed.

Unlike the original self-hosted implementation this does not use shared-daemon
port mutexes, PR-controlled Compose, a fixed `seekdb` container name, a privileged
C++ builder, or sudo to start database containers. Host ports come from Docker
and are passed directly to Go, avoiding the original sudo/environment propagation
issue. The Go command uses `CGO_ENABLED=0 go test -tags=integration` for this
focused storage test instead of the full native/C++ build wrapper. Consequently
this does not cover CGO tokenizer/native integration; future package changes
that require CGO will fail explicitly and need review.

When publishing the default-branch entrypoint, replace its reusable-workflow
`@seekdb-ci` reference with the **published implementation commit SHA**. The
sample branch reference is for staging only; pinning avoids accidentally changing
the scheduled implementation when editing the branch.
