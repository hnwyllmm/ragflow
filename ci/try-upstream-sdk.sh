#!/usr/bin/env bash
# Deliberately runs original SDK fixtures. Their engine/credential restrictions
# remain unchanged and appear as blocked/provisioning errors, never waived.
set -euo pipefail
root=$(pwd)
mkdir -p artifacts
export HOST_ADDRESS=http://127.0.0.1:9380 DOC_ENGINE=seekdb CI=true RAGFLOW_SDK_CI=1
# Empty values permit import of configs.py, but are NOT usable/fake credentials.
# The original provisioning fixture independently requires nonempty credentials.
export ZHIPU_AI_API_KEY='' SILICONFLOW_API_KEY=''
set +e
timeout 600 bash -c '
  set -euo pipefail
  python3 -m venv sdk-tools
  sdk-tools/bin/pip install uv==0.9.11
  sdk-tools/bin/uv venv --python 3.13 sdk-env
  sdk-tools/bin/uv pip install --python sdk-env/bin/python ./baseline/sdk/python \
    --group baseline/sdk/python/pyproject.toml:test \
    pytest-asyncio==1.3.0 pytest-xdist==3.8.0 pytest-cov==7.0.0
' > artifacts/sdk-setup.log 2>&1
setup_rc=$?
echo "$setup_rc" > artifacts/sdk-setup.exit
if (( setup_rc != 0 )); then
  echo 'SDK dependency setup blocked; no SDK test result claimed' > artifacts/sdk-status.txt
  cat artifacts/sdk-setup.log
  exit "$setup_rc"
fi
(cd baseline && timeout 180 "$root/sdk-env/bin/python" -m pytest --collect-only -q --level=p2 test/testcases/test_sdk_api) > artifacts/sdk-collection.log 2>&1
collect_rc=$?
echo "$collect_rc" > artifacts/sdk-collection.exit
(cd baseline && timeout 900 "$root/sdk-env/bin/python" -m pytest -s --tb=short --level=p2 \
  --junitxml="$root/artifacts/sdk.xml" test/testcases/test_sdk_api) > artifacts/sdk.log 2>&1
rc=$?
echo "$rc" > artifacts/sdk.exit
cat artifacts/sdk.log
if grep -qi 'SDK CI provisioning failed' artifacts/sdk.log; then
  echo 'BLOCKED: original upstream SDK provisioning rejects SeekDB and requires external model credentials' > artifacts/sdk-status.txt
elif (( rc != 0 )); then
  echo "SDK attempt failed (exit $rc); inspect sdk.log/sdk.xml, do not count as pass" > artifacts/sdk-status.txt
else
  echo 'SDK invocation completed; inspect sdk.xml for actual pass/skip counts; not parity' > artifacts/sdk-status.txt
fi
exit "$rc"
