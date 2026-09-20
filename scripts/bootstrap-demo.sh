#!/usr/bin/env bash
set -euo pipefail

base_url="${BASE_URL:-http://localhost:8080}"
admin_token="${ADMIN_TOKEN:-dev-admin-token-change-before-prod}"

command -v jq >/dev/null || { echo "jq is required" >&2; exit 1; }

api() {
  local method="$1" path="$2" body="${3:-}"
  if [[ -n "$body" ]]; then
    curl --fail --silent --show-error -X "$method" \
      -H "Authorization: Bearer $admin_token" \
      -H "Content-Type: application/json" \
      --data "$body" "$base_url$path"
  else
    curl --fail --silent --show-error -X "$method" \
      -H "Authorization: Bearer $admin_token" \
      -H "Content-Type: application/json" "$base_url$path"
  fi
}

user_a="$(api POST /api/v1/admin/users '{"username":"user_a","display_name":"Demo reader","password":"user_a_123"}' | jq -r .id)"
user_c="$(api POST /api/v1/admin/users '{"username":"user_c","display_name":"Demo writer","password":"user_c_123"}' | jq -r .id)"

create_resource() {
  local key="$1"
  api POST /api/v1/admin/resources "{\"resource_key\":\"$key\",\"display_name\":\"$key\",\"host\":\"target-mysql\",\"port\":3306,\"database_name\":\"$key\",\"read_username\":\"gateway_read\",\"read_secret_ref\":\"demo_read\",\"write_username\":\"gateway_write\",\"write_secret_ref\":\"demo_write\",\"tls_mode\":\"disabled\",\"max_rows\":1000,\"max_write_rows\":100,\"statement_timeout_ms\":10000,\"enabled\":true}" | jq -r .id
}

database_a="$(create_resource database_a)"
database_b="$(create_resource database_b)"
database_c="$(create_resource database_c)"

grant() {
  api POST /api/v1/admin/grants "{\"principal_id\":\"$1\",\"resource_id\":\"$2\",\"action\":\"$3\",\"row_limit\":200,\"statement_timeout_ms\":5000}" >/dev/null
}
grant "$user_a" "$database_a" query_read
grant "$user_a" "$database_b" schema_read
grant "$user_c" "$database_a" query_read
grant "$user_c" "$database_c" query_write

cookie_dir="$(mktemp -d)"
trap 'rm -rf "$cookie_dir"' EXIT

create_user_token() {
  local user_id="$1" username="$2" password="$3" cookie="$cookie_dir/$2.cookies"
  curl --fail --silent --show-error -c "$cookie" -X POST \
    -H "Content-Type: application/json" \
    --data "{\"username\":\"$username\",\"password\":\"$password\"}" "$base_url/api/v1/auth/login" >/dev/null
  curl --fail --silent --show-error -b "$cookie" -X POST \
    -H "Content-Type: application/json" \
    --data '{"name":"demo","expires_in_days":30}' "$base_url/api/v1/me/tokens" | jq -r .token
}

token_a="$(create_user_token "$user_a" user_a user_a_123)"
token_c="$(create_user_token "$user_c" user_c user_c_123)"

printf 'Demo data created. Tokens are shown once.\nUSER_A_TOKEN=%s\nUSER_C_TOKEN=%s\n' "$token_a" "$token_c"
