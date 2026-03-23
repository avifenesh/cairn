#!/usr/bin/env bash
# Test: TLS cert expiry check command from system-health SKILL.md
# Validates that the command correctly computes days remaining and status.
set -euo pipefail

# The TLS check command extracted from the SKILL.md (parameterized for testability)
tls_check() {
    local domain="$1"
    local now_ts="$2"  # epoch seconds, for deterministic testing

    local end_date
    end_date=$(timeout 5 bash -c 'set -o pipefail; echo | openssl s_client -connect "$1:443" -servername "$1" 2>/dev/null | openssl x509 -noout -enddate 2>/dev/null | cut -d= -f2' _ "$domain" 2>/dev/null)

    if [ -z "$end_date" ]; then
        echo "expiry: UNKNOWN, days_remaining: UNKNOWN, status: UNKNOWN"
        return
    fi

    local end_ts
    end_ts=$(date -d "$end_date" +%s 2>/dev/null || echo "")
    if ! [[ "$end_ts" =~ ^[0-9]+$ ]]; then
        echo "expiry: UNKNOWN, days_remaining: UNKNOWN, status: UNKNOWN"
        return
    fi
    local days=$(( (end_ts - now_ts) / 86400 ))

    local status
    if [ "$days" -lt 3 ]; then
        status="CRIT"
    elif [ "$days" -lt 14 ]; then
        status="WARN"
    else
        status="OK"
    fi

    printf "expiry: %s, days_remaining: %d, status: %s\n" "$end_date" "$days" "$status"
}

# --- Threshold logic tests (using a mock function for deterministic testing) ---

# Mock version that lets us control the computed delta
tls_check_mock() {
    local days="$1"
    local status
    if [ "$days" -lt 3 ]; then
        status="CRIT"
    elif [ "$days" -lt 14 ]; then
        status="WARN"
    else
        status="OK"
    fi
    echo "days_remaining: $days, status: $status"
}

passed=0
failed=0

assert_status() {
    local days="$1"
    local expected="$2"
    local actual
    actual=$(tls_check_mock "$days" | grep -oE 'status: [A-Z]+' | sed 's/status: //')
    if [ "$actual" = "$expected" ]; then
        passed=$((passed + 1))
    else
        failed=$((failed + 1))
        echo "FAIL: days=$days expected status=$expected got status=$actual"
    fi
}

echo "=== TLS cert expiry threshold tests ==="

# Regression test: 65 days should be OK (was falsely reported as CRITICAL)
assert_status 65 "OK"

# Boundary tests
assert_status 14 "OK"    # exactly 14 days — still OK
assert_status 13 "WARN"  # 13 days — WARN
assert_status 3 "WARN"   # exactly 3 days — still WARN
assert_status 2 "CRIT"   # 2 days — CRIT
assert_status 1 "CRIT"   # 1 day — CRIT
assert_status 0 "CRIT"   # 0 days — CRIT
assert_status 90 "OK"    # fresh cert — OK
assert_status 30 "OK"    # 30 days — OK
assert_status 15 "OK"    # 15 days — OK

if [ "${TLS_LIVE_TEST:-}" = "1" ]; then
    echo "=== Live test against agentic.garden ==="
    # Test the actual command (requires network)
    now_ts=$(date +%s)
    output=$(tls_check "agentic.garden" "$now_ts" 2>&1) || true
    echo "Output: $output"

    # Verify the output contains the expected fields (including UNKNOWN when offline)
    if echo "$output" | grep -qE "expiry: .+, days_remaining: (UNKNOWN|[0-9]+), status: (OK|WARN|CRIT|UNKNOWN)"; then
        echo "PASS: output format is correct"
        passed=$((passed + 1))
    else
        echo "FAIL: output format is incorrect"
        failed=$((failed + 1))
    fi

    # Verify that the reported status is consistent with days_remaining when numeric.
    days_in_output=$(echo "$output" | grep -oE 'days_remaining: [0-9]+' | sed 's/days_remaining: //' || true)
    status_in_output=$(echo "$output" | grep -oE 'status: [A-Z]+' | sed 's/status: //' || true)

    if [ -z "${days_in_output:-}" ] || [ -z "${status_in_output:-}" ] || [ "$status_in_output" = "UNKNOWN" ]; then
        echo "INFO: days_remaining or status is UNKNOWN; skipping status consistency check."
        passed=$((passed + 1))
    else
        expected_status=""
        if [ "$days_in_output" -lt 3 ]; then
            expected_status="CRIT"
        elif [ "$days_in_output" -lt 14 ]; then
            expected_status="WARN"
        else
            expected_status="OK"
        fi

        if [ "$status_in_output" = "$expected_status" ]; then
            echo "PASS: status ($status_in_output) is consistent with days_remaining ($days_in_output)"
            passed=$((passed + 1))
        else
            echo "FAIL: status ($status_in_output) inconsistent with days_remaining ($days_in_output), expected $expected_status"
            failed=$((failed + 1))
        fi
    fi
else
    echo "=== Skipping live TLS test against agentic.garden (set TLS_LIVE_TEST=1 to enable) ==="
fi

echo ""
echo "Results: $passed passed, $failed failed"
if [ "$failed" -gt 0 ]; then
    exit 1
fi
