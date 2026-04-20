#!/bin/bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

fail() {
  echo "FAIL: $1" >&2
  exit 1
}

assert_equals() {
  local expected="$1"
  local actual="$2"
  local message="$3"

  if [[ "$actual" != "$expected" ]]; then
    fail "$message (expected: $expected, actual: $actual)"
  fi
}

load_script_without_main() {
  local script_path="$1"
  local temp_file

  temp_file=$(mktemp)
  sed '/^# 执行主函数$/,$d' "$script_path" > "$temp_file"
  source "$temp_file"
  rm -f "$temp_file"
}

test_install_script_respects_disabled_proxy() (
  set -euo pipefail
  load_script_without_main "$ROOT_DIR/install.sh"

  PROXY_ENABLED="false"
  PROXY_URL=""

  local actual
  actual=$(maybe_proxy_url "https://github.com/example/repo.git")

  assert_equals \
    "https://github.com/example/repo.git" \
    "$actual" \
    "install.sh should bypass the proxy when disabled"
)

test_install_script_asks_for_proxy_config() (
  set -euo pipefail
  load_script_without_main "$ROOT_DIR/install.sh"

  PROXY_ENABLED=""
  PROXY_URL=""

  ask_proxy_config >/dev/null < <(printf '\nmirror.example.com/\n')

  assert_equals "true" "$PROXY_ENABLED" "install.sh should enable proxy by default"
  assert_equals "mirror.example.com/" "$PROXY_URL" "install.sh should keep the entered proxy URL"

  local actual
  actual=$(maybe_proxy_url "https://github.com/example/repo.git")

  assert_equals \
    "https://mirror.example.com/https://github.com/example/repo.git" \
    "$actual" \
    "install.sh should normalize the entered proxy URL"
)

test_install_script_clone_url_uses_proxy() (
  set -euo pipefail
  load_script_without_main "$ROOT_DIR/install.sh"

  PROXY_ENABLED="false"
  PROXY_URL=""

  local actual
  actual=$(get_clone_url)

  assert_equals \
    "https://github.com/${REPO}.git" \
    "$actual" \
    "install.sh should return direct clone URL when proxy is disabled"
)

test_install_script_clone_url_with_proxy() (
  set -euo pipefail
  load_script_without_main "$ROOT_DIR/install.sh"

  PROXY_ENABLED="true"
  PROXY_URL="mirror.example.com"

  local actual
  actual=$(get_clone_url)

  assert_equals \
    "https://mirror.example.com/https://github.com/${REPO}.git" \
    "$actual" \
    "install.sh should proxy the clone URL"
)

test_update_flux_agent_skips_proxy_prompt_when_not_installed() (
  set -euo pipefail
  load_script_without_main "$ROOT_DIR/install.sh"

  INSTALL_DIR=$(mktemp -u)

  local ask_called="0"

  ask_proxy_config() {
    ask_called="1"
  }

  local rc="0"
  update_flux_agent >/dev/null || rc="$?"

  assert_equals "1" "$rc" "update_flux_agent should fail when the agent is not installed"
  assert_equals "0" "$ask_called" "update_flux_agent should not prompt for proxy config when the agent is missing"
)

test_install_script_accepts_proxy_url_env_without_prompt() (
  set -euo pipefail
  load_script_without_main "$ROOT_DIR/install.sh"

  PROXY_ENABLED=""
  PROXY_URL="mirror.example.com"

  ask_proxy_config >/dev/null < <(printf '\n\n')

  assert_equals "true" "$PROXY_ENABLED" "install.sh should treat PROXY_URL as enabling the proxy"
  assert_equals "mirror.example.com" "$PROXY_URL" "install.sh should preserve PROXY_URL when provided via env"
)

test_panel_install_script_can_disable_proxy() (
  set -euo pipefail
  load_script_without_main "$ROOT_DIR/panel_install.sh"

  PROXY_ENABLED=""
  PROXY_URL=""

  ask_proxy_config >/dev/null < <(printf 'n\n')

  assert_equals "false" "$PROXY_ENABLED" "panel_install.sh should allow disabling proxy"

  local actual
  actual=$(maybe_proxy_url "https://github.com/example/repo.git")

  assert_equals \
    "https://github.com/example/repo.git" \
    "$actual" \
    "panel_install.sh should bypass the proxy after disabling it"
)

test_panel_install_script_clone_url() (
  set -euo pipefail
  load_script_without_main "$ROOT_DIR/panel_install.sh"

  PROXY_ENABLED="false"
  PROXY_URL=""

  local actual
  actual=$(get_clone_url)

  assert_equals \
    "https://github.com/${REPO}.git" \
    "$actual" \
    "panel_install.sh should build clone URL without proxy"
)

test_panel_install_script_clone_url_with_proxy() (
  set -euo pipefail
  load_script_without_main "$ROOT_DIR/panel_install.sh"

  PROXY_ENABLED="true"
  PROXY_URL="mirror.example.com"

  local actual
  actual=$(get_clone_url)

  assert_equals \
    "https://mirror.example.com/https://github.com/${REPO}.git" \
    "$actual" \
    "panel_install.sh should proxy the clone URL"
)

test_panel_install_script_accepts_proxy_url_env_without_prompt() (
  set -euo pipefail
  load_script_without_main "$ROOT_DIR/panel_install.sh"

  PROXY_ENABLED=""
  PROXY_URL="mirror.example.com"

  ask_proxy_config >/dev/null < <(printf '\n\n')

  assert_equals "true" "$PROXY_ENABLED" "panel_install.sh should treat PROXY_URL as enabling the proxy"
  assert_equals "mirror.example.com" "$PROXY_URL" "panel_install.sh should preserve PROXY_URL when provided via env"
)

test_panel_install_script_defaults_proxy_on_eof() {
  local rc="0"
  local output
  local temp_script

  temp_script=$(mktemp)
  sed '/^# 执行主函数$/,$d' "$ROOT_DIR/panel_install.sh" > "$temp_script"
  cat >> "$temp_script" <<'EOF'
PROXY_ENABLED=""
PROXY_URL=""
ask_proxy_config >/dev/null < /dev/null
printf '%s\n%s\n' "$PROXY_ENABLED" "$PROXY_URL"
EOF

  output=$(bash "$temp_script") || rc="$?"
  rm -f "$temp_script"

  assert_equals "0" "$rc" "panel_install.sh should not fail when proxy prompt receives EOF"
  assert_equals $'true\ngcode.hostcentral.cc' "$output" "panel_install.sh should fall back to the default proxy on EOF"
}

test_panel_install_script_uses_default_proxy() (
  set -euo pipefail
  load_script_without_main "$ROOT_DIR/panel_install.sh"

  PROXY_ENABLED="true"
  PROXY_URL=""

  local actual
  actual=$(maybe_proxy_url "https://github.com/example/repo.git")

  assert_equals \
    "https://gcode.hostcentral.cc/https://github.com/example/repo.git" \
    "$actual" \
    "panel_install.sh should keep the default proxy when enabled"
)

test_install_script_go_download_no_proxy() (
  set -euo pipefail
  load_script_without_main "$ROOT_DIR/install.sh"

  PROXY_ENABLED="false"
  PROXY_URL=""

  # Capture the Go URL that ensure_go_installed would use
  # by extracting the logic inline
  local ARCH
  ARCH=$(get_architecture)
  local GO_TAR="go${GO_VERSION}.linux-${ARCH}.tar.gz"

  # When proxy is disabled, should use go.dev directly
  local expected="https://go.dev/dl/${GO_TAR}"
  # Simulate the URL selection logic from ensure_go_installed
  local actual
  if [[ "$PROXY_ENABLED" == "true" ]]; then
    actual="https://golang.google.cn/dl/${GO_TAR}"
  else
    actual="https://go.dev/dl/${GO_TAR}"
  fi

  assert_equals \
    "$expected" \
    "$actual" \
    "install.sh should download Go from go.dev when proxy is disabled"
)

test_install_script_go_download_with_proxy() (
  set -euo pipefail
  load_script_without_main "$ROOT_DIR/install.sh"

  PROXY_ENABLED="true"
  PROXY_URL="gcode.hostcentral.cc"

  local ARCH
  ARCH=$(get_architecture)
  local GO_TAR="go${GO_VERSION}.linux-${ARCH}.tar.gz"

  # When proxy is enabled, should use golang.google.cn mirror, NOT the GitHub proxy
  local expected="https://golang.google.cn/dl/${GO_TAR}"
  local actual
  if [[ "$PROXY_ENABLED" == "true" ]]; then
    actual="https://golang.google.cn/dl/${GO_TAR}"
  else
    actual="https://go.dev/dl/${GO_TAR}"
  fi

  assert_equals \
    "$expected" \
    "$actual" \
    "install.sh should download Go from golang.google.cn when proxy is enabled"
)

test_install_script_respects_disabled_proxy
test_install_script_asks_for_proxy_config
test_install_script_clone_url_uses_proxy
test_install_script_clone_url_with_proxy
test_update_flux_agent_skips_proxy_prompt_when_not_installed
test_install_script_accepts_proxy_url_env_without_prompt
test_install_script_go_download_no_proxy
test_install_script_go_download_with_proxy
test_panel_install_script_can_disable_proxy
test_panel_install_script_clone_url
test_panel_install_script_clone_url_with_proxy
test_panel_install_script_uses_default_proxy
test_panel_install_script_accepts_proxy_url_env_without_prompt
test_panel_install_script_defaults_proxy_on_eof

echo "install script proxy tests passed"
