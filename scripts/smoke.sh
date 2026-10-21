#!/usr/bin/env bash
# End-to-end smoke test: account -> category -> transactions -> balance.
# Requires curl, python3, and a running stack (docker compose up).
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
NAME_SUFFIX="$(date +%s)-$$"

json_field() {
	python3 -c 'import json,sys; print(json.load(sys.stdin)[sys.argv[1]])' "$1"
}

account_id=""
category_id=""
transaction_id=""
second_id=""

cleanup() {
	for id in "$transaction_id" "$second_id"; do
		[ -n "$id" ] && curl -fsS -X DELETE "$BASE_URL/transactions/$id" >/dev/null || true
	done
	[ -n "$category_id" ] && curl -fsS -X DELETE "$BASE_URL/categories/$category_id" >/dev/null || true
	[ -n "$account_id" ] && curl -fsS -X DELETE "$BASE_URL/accounts/$account_id" >/dev/null || true
}
trap cleanup EXIT

echo "checking health"
curl -fsS "$BASE_URL/healthz" >/dev/null

echo "creating account and category"
account_id=$(curl -fsS -X POST "$BASE_URL/accounts" \
	-H 'Content-Type: application/json' \
	-d '{"name":"Smoke Checking","type":"checking"}' | json_field id)
category_id=$(curl -fsS -X POST "$BASE_URL/categories" \
	-H 'Content-Type: application/json' \
	-d "{\"name\":\"Smoke Groceries $NAME_SUFFIX\",\"kind\":\"expense\"}" | json_field id)

echo "creating transactions"
transaction_id=$(curl -fsS -X POST "$BASE_URL/transactions" \
	-H 'Content-Type: application/json' \
	-d "{\"account_id\":$account_id,\"category_id\":$category_id,\"amount_cents\":-1250,\"description\":\"smoke out\",\"occurred_on\":\"2026-01-20\"}" | json_field id)
second_id=$(curl -fsS -X POST "$BASE_URL/transactions" \
	-H 'Content-Type: application/json' \
	-d "{\"account_id\":$account_id,\"amount_cents\":2500,\"description\":\"smoke in\",\"occurred_on\":\"2026-01-21\"}" | json_field id)

echo "checking balance"
balance=$(curl -fsS "$BASE_URL/accounts/$account_id/balance" | json_field balance_cents)
if [ "$balance" != "1250" ]; then
	echo "balance = $balance, want 1250" >&2
	exit 1
fi

echo "checking transaction list"
count=$(curl -fsS "$BASE_URL/transactions?account_id=$account_id&from=2026-01-01&to=2026-01-31" | python3 -c 'import json,sys; print(len(json.load(sys.stdin)))')
if [ "$count" != "2" ]; then
	echo "transactions = $count, want 2" >&2
	exit 1
fi

echo "smoke test passed"
