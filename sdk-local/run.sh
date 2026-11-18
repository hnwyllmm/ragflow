#!/usr/bin/env bash
# Run from workflow workspace. Baseline/server/env are prepared independently.
set -euo pipefail
root=$(pwd)
here=$(cd "$(dirname "$0")" && pwd)
python=${SDK_PYTHON:-$root/sdk-env/bin/python}
[[ -x "$python" ]]
mkdir -p artifacts/sdk-local
# Select named folder groups; default is the smallest metadata-only start.
groups=("${@:-dataset}")
if [[ ${groups[*]} == all ]]; then groups=(dataset document chunk); fi
status=0
for group in "${groups[@]}"; do
  case "$group" in
    dataset) folder=test_dataset_mangement; expected=46;;
    document) folder=test_file_management_within_dataset; expected=98;;
    chunk) folder=test_chunk_management_within_dataset; expected=11;;
    *) echo "Unknown SDK group: $group" >&2; exit 2;;
  esac
  out="$root/artifacts/sdk-local/$group"
  mkdir -p "$out"
  private=$(mktemp -d)
  trap 'rm -rf "$private"' EXIT
  set +e
  timeout 180 go run "$here/bootstrap.go" http://127.0.0.1:9380 "$root/baseline/test/testcases/configs.py" "$private/env.json" > "$out/bootstrap.log" 2>&1
  bootstrap_rc=$?
  set -e
  echo "$bootstrap_rc" > "$out/bootstrap.exit"
  if (( bootstrap_rc != 0 )); then
    echo "BLOCKED bootstrap: $group; no SDK assertions executed" > "$out/status.txt"
    status=1
    rm -rf "$private"
    continue
  fi
  # Export only explicitly recognized fields, never evaluate shell from JSON.
  for name in RAGFLOW_SDK_ISOLATED RAGFLOW_SDK_BASE_URL RAGFLOW_SDK_API_KEY RAGFLOW_SDK_TENANT_ID RAGFLOW_SDK_OTHER_API_KEY RAGFLOW_SDK_OTHER_TENANT_ID RAGFLOW_SDK_DOC_ENGINE RAGFLOW_SDK_EMBEDDING_MODEL; do
    value=$(jq -er --arg key "$name" '.[$key]' "$private/env.json")
    if [[ ${GITHUB_ACTIONS:-} == true && "$name" == *_API_KEY ]]; then
      printf '::add-mask::%s\n' "$value"
    fi
    export "$name=$value"
  done
  export HOST_ADDRESS=http://127.0.0.1:9380 DOC_ENGINE=seekdb
  export RAGFLOW_SDK_CI=0 ZHIPU_AI_API_KEY='' SILICONFLOW_API_KEY=''
  # Disable only upstream automatic provisioning via its supported caller-owned
  # environment switch. Original tests, assertion/cleanup fixtures and p2 filter remain.
  set +e
  (cd "$root/baseline" && timeout 900 "$python" -m pytest -s --tb=short --level=p2 \
    --junitxml="$out/results.xml" "test/testcases/test_sdk_api/$folder") > "$out/tests.log" 2>&1
  rc=$?
  set -e
  echo "$rc" > "$out/tests.exit"
  echo "group=$group expected_p2_integration=$expected pytest_exit=$rc (contract cases counted separately)" > "$out/status.txt"
  set +e
  go run "$here/report.go" "$here/cases-dc8768a.json" "$out/results.xml" "$folder" > "$out/summary.txt" 2>&1
  report_rc=$?
  set -e
  cat "$out/status.txt" "$out/summary.txt"
  if (( report_rc != 0 )); then status=1; fi
  if (( rc != 0 )); then status=1; fi
  rm -rf "$private"
done
exit "$status"
