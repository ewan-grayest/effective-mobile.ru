#!/usr/bin/env bash
# End-to-end API test. Requires: jq, curl, the service running on $BASE.
#
#   docker compose up -d --build
#   ./test_api.sh

set -u

BASE="${BASE:-http://localhost:8080/api/v1}"
USER_A="60601fee-2bf1-4721-ae6f-7636e79a0cba"
USER_B="aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

PASS=0
FAIL=0
RED=$'\033[31m'; GRN=$'\033[32m'; RST=$'\033[0m'

# expect_status METHOD URL EXPECTED_CODE [BODY] -- prints PASS/FAIL
expect_status() {
    local method="$1" url="$2" expected="$3" body="${4:-}"
    local actual
    if [ -n "$body" ]; then
        actual=$(curl -s -o /dev/null -w "%{http_code}" -X "$method" "$url" \
            -H "Content-Type: application/json" -d "$body")
    else
        actual=$(curl -s -o /dev/null -w "%{http_code}" -X "$method" "$url")
    fi
    if [ "$actual" = "$expected" ]; then
        echo "${GRN}PASS${RST} $method $url → $actual"
        PASS=$((PASS + 1))
    else
        echo "${RED}FAIL${RST} $method $url → expected $expected, got $actual"
        FAIL=$((FAIL + 1))
    fi
}

# expect_field METHOD URL JQ_EXPR EXPECTED [BODY]
expect_field() {
    local method="$1" url="$2" jq_expr="$3" expected="$4" body="${5:-}"
    local resp actual
    if [ -n "$body" ]; then
        resp=$(curl -s -X "$method" "$url" -H "Content-Type: application/json" -d "$body")
    else
        resp=$(curl -s -X "$method" "$url")
    fi
    actual=$(echo "$resp" | jq -r "$jq_expr")
    if [ "$actual" = "$expected" ]; then
        echo "${GRN}PASS${RST} $method $url $jq_expr = $actual"
        PASS=$((PASS + 1))
    else
        echo "${RED}FAIL${RST} $method $url $jq_expr — expected $expected, got $actual"
        echo "       response: $resp"
        FAIL=$((FAIL + 1))
    fi
}

echo "── Cleanup: deleting all existing subscriptions ──────────────"
for id in $(curl -s "$BASE/subscriptions" | jq -r '.[].id'); do
    curl -s -o /dev/null -X DELETE "$BASE/subscriptions/$id"
done

echo
echo "── Create (happy path) — keep IDs of both records ────────────"
# Existing dates 07-2025 / 01-2025 are in the past relative to today, so the
# strict-validation check requires allow_behindhand_date: true to keep the
# previously-asserted Total numbers stable.
RESP_A=$(curl -s -X POST "$BASE/subscriptions" -H "Content-Type: application/json" \
    -d "{\"service_name\":\"Yandex Plus\",\"price\":400,\"user_id\":\"$USER_A\",\"start_date\":\"07-2025\",\"allow_behindhand_date\":true}")
SUB_A=$(echo "$RESP_A" | jq -r '.id')
[ -n "$SUB_A" ] && [ "$SUB_A" != "null" ] \
    && { echo "${GRN}PASS${RST} created A id=$SUB_A"; PASS=$((PASS+1)); } \
    || { echo "${RED}FAIL${RST} no id in response: $RESP_A"; FAIL=$((FAIL+1)); }

RESP_B=$(curl -s -X POST "$BASE/subscriptions" -H "Content-Type: application/json" \
    -d "{\"service_name\":\"Netflix\",\"price\":799,\"user_id\":\"$USER_A\",\"start_date\":\"01-2025\",\"end_date\":\"12-2025\",\"allow_behindhand_date\":true}")
SUB_B=$(echo "$RESP_B" | jq -r '.id')
[ -n "$SUB_B" ] && [ "$SUB_B" != "null" ] \
    && { echo "${GRN}PASS${RST} created B id=$SUB_B"; PASS=$((PASS+1)); } \
    || { echo "${RED}FAIL${RST} no id in response: $RESP_B"; FAIL=$((FAIL+1)); }

# keep $SUB_ID as alias for backward-compatibility with later sections
SUB_ID="$SUB_A"

echo
echo "── Create — validation errors (must all return 400) ─────────"
expect_status POST "$BASE/subscriptions" 400 \
    "{\"price\":400,\"user_id\":\"$USER_A\",\"start_date\":\"07-2025\",\"allow_behindhand_date\":true}"
expect_status POST "$BASE/subscriptions" 400 \
    "{\"service_name\":\"X\",\"price\":0,\"user_id\":\"$USER_A\",\"start_date\":\"07-2025\",\"allow_behindhand_date\":true}"
expect_status POST "$BASE/subscriptions" 400 \
    "{\"service_name\":\"X\",\"price\":100,\"user_id\":\"not-uuid\",\"start_date\":\"07-2025\",\"allow_behindhand_date\":true}"
expect_status POST "$BASE/subscriptions" 400 \
    "{\"service_name\":\"X\",\"price\":100,\"user_id\":\"$USER_A\"}"
expect_status POST "$BASE/subscriptions" 400 \
    "{\"service_name\":\"X\",\"price\":100,\"user_id\":\"$USER_A\",\"start_date\":\"2025-07\"}"
expect_status POST "$BASE/subscriptions" 400 \
    "{\"service_name\":\"X\",\"price\":100,\"user_id\":\"$USER_A\",\"start_date\":\"06-2025\",\"end_date\":\"01-2025\",\"allow_behindhand_date\":true}"
expect_status POST "$BASE/subscriptions" 400 'not-json'

echo
echo "── Strict validation rules ──────────────────────────────────"
# Missing Content-Type → 415
ACTUAL=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE/subscriptions" \
    -d "{\"service_name\":\"X\",\"price\":1,\"user_id\":\"$USER_A\",\"start_date\":\"07-2025\",\"allow_behindhand_date\":true}")
if [ "$ACTUAL" = "415" ]; then
    echo "${GRN}PASS${RST} missing Content-Type → 415"; PASS=$((PASS+1))
else
    echo "${RED}FAIL${RST} missing Content-Type → expected 415, got $ACTUAL"; FAIL=$((FAIL+1))
fi

# Unknown field → 400
expect_status POST "$BASE/subscriptions" 400 \
    "{\"service_nam\":\"X\",\"price\":1,\"user_id\":\"$USER_A\",\"start_date\":\"07-2025\",\"allow_behindhand_date\":true}"

# Duplicate field → 400
expect_status POST "$BASE/subscriptions" 400 \
    "{\"service_name\":\"A\",\"service_name\":\"B\",\"price\":1,\"user_id\":\"$USER_A\",\"start_date\":\"07-2025\",\"allow_behindhand_date\":true}"

# Whitespace-only service_name → 400
expect_status POST "$BASE/subscriptions" 400 \
    "{\"service_name\":\"   \",\"price\":1,\"user_id\":\"$USER_A\",\"start_date\":\"07-2025\",\"allow_behindhand_date\":true}"

# service_name > 255 chars → 400
LONG_NAME=$(printf 'X%.0s' $(seq 1 300))
expect_status POST "$BASE/subscriptions" 400 \
    "{\"service_name\":\"$LONG_NAME\",\"price\":1,\"user_id\":\"$USER_A\",\"start_date\":\"07-2025\",\"allow_behindhand_date\":true}"

# Past start_date without allow flag → 400
expect_status POST "$BASE/subscriptions" 400 \
    "{\"service_name\":\"X\",\"price\":1,\"user_id\":\"$USER_A\",\"start_date\":\"01-2020\"}"

# Past start_date WITH allow flag → 201
RESP_C=$(curl -s -w "\n%{http_code}" -X POST "$BASE/subscriptions" -H "Content-Type: application/json" \
    -d "{\"service_name\":\"Backdated\",\"price\":1,\"user_id\":\"$USER_A\",\"start_date\":\"01-2020\",\"allow_behindhand_date\":true}")
STATUS_C=$(echo "$RESP_C" | tail -n1)
BODY_C=$(echo "$RESP_C" | sed '$d')
if [ "$STATUS_C" = "201" ]; then
    echo "${GRN}PASS${RST} past start_date with allow flag → 201"; PASS=$((PASS+1))
    SUB_C=$(echo "$BODY_C" | jq -r '.id')
    # immediately delete it so it doesn't pollute Total assertions below
    curl -s -o /dev/null -X DELETE "$BASE/subscriptions/$SUB_C"
else
    echo "${RED}FAIL${RST} past start_date with allow flag → expected 201, got $STATUS_C"; FAIL=$((FAIL+1))
fi

# end_date == start_date → valid 1-month subscription → 201
# Verify via total: price=400 × 1 month = 400
RESP_M=$(curl -s -w "\n%{http_code}" -X POST "$BASE/subscriptions" -H "Content-Type: application/json" \
    -d "{\"service_name\":\"OneMonth\",\"price\":400,\"user_id\":\"$USER_A\",\"start_date\":\"03-2025\",\"end_date\":\"03-2025\",\"allow_behindhand_date\":true}")
STATUS_M=$(echo "$RESP_M" | tail -n1)
BODY_M=$(echo "$RESP_M" | sed '$d')
if [ "$STATUS_M" = "201" ]; then
    echo "${GRN}PASS${RST} 1-month subscription (start_date == end_date) → 201"; PASS=$((PASS+1))
    SUB_M=$(echo "$BODY_M" | jq -r '.id')
    # Confirm total math: query Mar-2025..Mar-2025 with service_name filter
    expect_field GET "$BASE/subscriptions/total?from=03-2025&to=03-2025&service_name=OneMonth" '.total' '400'
    curl -s -o /dev/null -X DELETE "$BASE/subscriptions/$SUB_M"
else
    echo "${RED}FAIL${RST} 1-month subscription → expected 201, got $STATUS_M (body: $BODY_M)"; FAIL=$((FAIL+1))
fi

# end_date strictly before start_date is still rejected → 400
expect_status POST "$BASE/subscriptions" 400 \
    "{\"service_name\":\"X\",\"price\":1,\"user_id\":\"$USER_A\",\"start_date\":\"06-2025\",\"end_date\":\"05-2025\",\"allow_behindhand_date\":true}"

echo
echo "── Get by ID — both records ─────────────────────────────────"
expect_status GET  "$BASE/subscriptions/$SUB_A" 200
expect_field  GET  "$BASE/subscriptions/$SUB_A" '.service_name' 'Yandex Plus'
expect_field  GET  "$BASE/subscriptions/$SUB_A" '.price'        '400'
expect_field  GET  "$BASE/subscriptions/$SUB_A" '.start_date'   '07-2025'

expect_status GET  "$BASE/subscriptions/$SUB_B" 200
expect_field  GET  "$BASE/subscriptions/$SUB_B" '.service_name' 'Netflix'
expect_field  GET  "$BASE/subscriptions/$SUB_B" '.price'        '799'
expect_field  GET  "$BASE/subscriptions/$SUB_B" '.end_date'     '12-2025'

expect_status GET  "$BASE/subscriptions/00000000-0000-0000-0000-000000000000" 404
expect_status GET  "$BASE/subscriptions/not-a-uuid" 400

echo
echo "── List with filters ────────────────────────────────────────"
expect_status GET "$BASE/subscriptions" 200
expect_field  GET "$BASE/subscriptions" 'length' '2'
expect_field  GET "$BASE/subscriptions?user_id=$USER_A"      'length' '2'
expect_field  GET "$BASE/subscriptions?user_id=$USER_B"      'length' '0'
expect_field  GET "$BASE/subscriptions?service_name=Netflix" 'length' '1'
expect_status GET "$BASE/subscriptions?user_id=not-uuid"     400

echo
echo "── Update — both records ────────────────────────────────────"
expect_field PUT "$BASE/subscriptions/$SUB_A" '.price'        '999'              '{"price":999}'
expect_field GET "$BASE/subscriptions/$SUB_A" '.start_date'   '07-2025'                                   # не должен измениться
expect_field PUT "$BASE/subscriptions/$SUB_B" '.service_name' 'Netflix Premium' '{"service_name":"Netflix Premium"}'
expect_field GET "$BASE/subscriptions/$SUB_B" '.price'        '799'                                       # цена тоже без изменений
expect_status PUT "$BASE/subscriptions/00000000-0000-0000-0000-000000000000" 404 '{"price":1}'
expect_status PUT "$BASE/subscriptions/$SUB_A" 400 '{"price":-1}'

# Strict validation also applies to PUT
expect_status PUT "$BASE/subscriptions/$SUB_A" 400 '{"service_nam":"X"}'                 # unknown field
expect_status PUT "$BASE/subscriptions/$SUB_A" 400 '{"service_name":"   "}'              # whitespace-only
expect_status PUT "$BASE/subscriptions/$SUB_A" 400 '{"start_date":"01-2020"}'            # past date without flag
expect_field  PUT "$BASE/subscriptions/$SUB_A" '.start_date' '01-2020' \
    '{"start_date":"01-2020","allow_behindhand_date":true}'                              # past date with flag → ok
# Restore SUB_A.start_date so the Total-cost section below uses the original value (07-2025).
curl -s -o /dev/null -X PUT "$BASE/subscriptions/$SUB_A" \
    -H "Content-Type: application/json" \
    -d '{"start_date":"07-2025","allow_behindhand_date":true}'

echo
echo "── Total cost ───────────────────────────────────────────────"
# State after creates and updates:
#   A = Yandex Plus, price=999 (updated), start=07-2025, no end_date
#   B = Netflix Premium (renamed), price=799, start=01-2025, end=12-2025
#
# Period 01-2025..12-2025 = A:999×6 + B:799×12 = 5994 + 9588 = 15582
expect_field GET "$BASE/subscriptions/total?from=01-2025&to=12-2025" '.total' '15582'

# Only the renamed Netflix Premium
expect_field GET "$BASE/subscriptions/total?from=01-2025&to=12-2025&service_name=Netflix%20Premium" '.total' '9588'

# Filter by USER_B — no records
expect_field GET "$BASE/subscriptions/total?from=01-2025&to=12-2025&user_id=$USER_B" '.total' '0'

# Single month 04-2025: only B is active (A starts only in 07-2025) → 799
expect_field GET "$BASE/subscriptions/total?from=04-2025&to=04-2025" '.total' '799'

# end_date_calc_mode filter
# A is open-ended (no end_date), B has end_date=12-2025.
# Period 01-2025..12-2025:
#   mode=all              → 15582 (A + B)
#   mode=with_end_date    →  9588 (only B: 799 × 12)
#   mode=without_end_date →  5994 (only A: 999 × 6)
expect_field GET "$BASE/subscriptions/total?from=01-2025&to=12-2025&end_date_calc_mode=all"              '.total' '15582'
expect_field GET "$BASE/subscriptions/total?from=01-2025&to=12-2025&end_date_calc_mode=with_end_date"    '.total' '9588'
expect_field GET "$BASE/subscriptions/total?from=01-2025&to=12-2025&end_date_calc_mode=without_end_date" '.total' '5994'

# Empty value of end_date_calc_mode is treated as "all"
expect_field GET "$BASE/subscriptions/total?from=01-2025&to=12-2025&end_date_calc_mode=" '.total' '15582'

# Unknown value → 400
expect_status GET "$BASE/subscriptions/total?from=01-2025&to=12-2025&end_date_calc_mode=bogus" 400

expect_status GET "$BASE/subscriptions/total"                         400
expect_status GET "$BASE/subscriptions/total?from=01-2025"            400
expect_status GET "$BASE/subscriptions/total?from=12-2025&to=01-2025" 400

echo
echo "── Delete — both records, then verify table is empty ────────"
expect_status DELETE "$BASE/subscriptions/$SUB_A" 204
expect_status GET    "$BASE/subscriptions/$SUB_A" 404
expect_status DELETE "$BASE/subscriptions/$SUB_A" 404                                                   # повторный delete — уже 404

expect_status DELETE "$BASE/subscriptions/$SUB_B" 204
expect_status GET    "$BASE/subscriptions/$SUB_B" 404

expect_field  GET    "$BASE/subscriptions" 'length' '0'                                                # таблица пустая

expect_status DELETE "$BASE/subscriptions/00000000-0000-0000-0000-000000000000" 404

echo
echo "── Methods not allowed ──────────────────────────────────────"
expect_status PATCH "$BASE/subscriptions" 405

echo
echo "─────────────────────────────────────────────────────────────"
echo "Passed: ${GRN}$PASS${RST}    Failed: ${RED}$FAIL${RST}"
[ "$FAIL" = 0 ] && exit 0 || exit 1
