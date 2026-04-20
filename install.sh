#!/bin/bash

# GitHub 仓库地址
REPO="Xeloan/flvx-tcppreconn"
BRANCH="main"
REPO_URL="https://github.com/${REPO}.git"

# Go 版本（用于自动安装）
GO_VERSION="1.25.0"

# 获取系统架构
get_architecture() {
    ARCH=$(uname -m)
    case $ARCH in
        x86_64)
            echo "amd64"
            ;;
        aarch64|arm64)
            echo "arm64"
            ;;
        *)
            echo "amd64"  # 默认使用 amd64
            ;;
    esac
}

# 安装目录
INSTALL_DIR="/etc/flux_agent"
# 源码目录
SRC_DIR="/opt/flvx-agent-src"

# 镜像加速配置（可由面板传入或交互式询问）
PROXY_ENABLED="${PROXY_ENABLED:-}"
PROXY_URL="${PROXY_URL:-}"

# 镜像加速
maybe_proxy_url() {
  local url="$1"

  if [[ "$PROXY_ENABLED" == "false" ]]; then
    echo "$url"
    return
  fi

  local proxy="${PROXY_URL:-gcode.hostcentral.cc}"

  if [[ "$proxy" == https://* || "$proxy" == http://* ]]; then
    proxy="${proxy%/}"
  else
    proxy="https://${proxy%/}"
  fi

  echo "${proxy}/${url}"
}

ask_proxy_config() {
  if [[ -n "$PROXY_ENABLED" ]]; then
    return
  fi

  if [[ -n "$PROXY_URL" ]]; then
    PROXY_ENABLED="true"
    return
  fi

  echo ""
  echo "==============================================="
  echo "           GitHub 加速配置"
  echo "==============================================="
  if ! read -r -p "是否开启 GitHub 加速? (Y/n): " proxy_choice; then
    proxy_choice=""
  fi
  case "$proxy_choice" in
    n|N)
      PROXY_ENABLED="false"
      echo "已关闭加速，将直连 GitHub"
      ;;
    *)
      PROXY_ENABLED="true"
      if ! read -r -p "加速地址 (默认 gcode.hostcentral.cc): " input_url; then
        input_url=""
      fi
      PROXY_URL="${input_url:-gcode.hostcentral.cc}"
      echo "已开启加速: $PROXY_URL"
      ;;
  esac
  echo "==============================================="
}

# 获取 clone URL（支持镜像加速）
get_clone_url() {
  maybe_proxy_url "$REPO_URL"
}

# 检查是否安装了 git
check_git() {
  if ! command -v git &> /dev/null; then
    echo "📦 安装 git..."
    if command -v apt-get &> /dev/null; then
      apt-get update -qq && apt-get install -y -qq git
    elif command -v yum &> /dev/null; then
      yum install -y -q git
    elif command -v dnf &> /dev/null; then
      dnf install -y -q git
    elif command -v apk &> /dev/null; then
      apk add --no-cache git
    else
      echo "❌ 无法自动安装 git，请手动安装"
      exit 1
    fi
  fi
}

# 检查并安装 Go 编译器
ensure_go_installed() {
  if command -v go &> /dev/null; then
    echo "✅ 检测到 Go: $(go version)"
    return 0
  fi

  echo "📦 安装 Go ${GO_VERSION}..."
  local ARCH=$(get_architecture)
  local GO_TAR="go${GO_VERSION}.linux-${ARCH}.tar.gz"
  local GO_URL
  # Use golang.google.cn mirror when proxy is enabled (for China/restricted networks)
  # The GitHub proxy (gcode.hostcentral.cc) only supports GitHub URLs, not go.dev
  if [[ "$PROXY_ENABLED" == "true" ]]; then
    GO_URL="https://golang.google.cn/dl/${GO_TAR}"
  else
    GO_URL="https://go.dev/dl/${GO_TAR}"
  fi

  if ! curl -fsSL "$GO_URL" -o "/tmp/${GO_TAR}"; then
    echo "❌ 下载 Go 失败"
    return 1
  fi
  rm -rf /usr/local/go
  if ! tar -C /usr/local -xzf "/tmp/${GO_TAR}"; then
    echo "❌ 解压 Go 失败"
    rm -f "/tmp/${GO_TAR}"
    return 1
  fi
  rm -f "/tmp/${GO_TAR}"

  export PATH="/usr/local/go/bin:$PATH"
  echo "✅ Go 安装完成: $(go version)"
}

# 检查并安装 C 编译器（用于编译 tcp_pool）
ensure_gcc_installed() {
  if command -v gcc &> /dev/null; then
    return 0
  fi

  echo "📦 安装 gcc..."
  if command -v apt-get &> /dev/null; then
    apt-get update -qq && apt-get install -y -qq gcc
  elif command -v yum &> /dev/null; then
    yum install -y -q gcc
  elif command -v dnf &> /dev/null; then
    dnf install -y -q gcc
  elif command -v apk &> /dev/null; then
    apk add --no-cache gcc musl-dev
  else
    echo "⚠️ 无法自动安装 gcc，tcp_pool 将不可用"
    return 1
  fi
}

# 克隆或更新仓库
clone_or_pull_repo() {
  local clone_url
  clone_url=$(get_clone_url)

  if [[ -d "$SRC_DIR/.git" ]]; then
    # Verify the existing repo's origin matches the expected URL.
    # If the user previously installed from a different fork (e.g. Sagit-chu/flvx),
    # the old origin would still point there, and git fetch/reset would pull old code.
    local current_origin
    current_origin=$(cd "$SRC_DIR" && git remote get-url origin 2>/dev/null || true)
    if [[ "$current_origin" == *"${REPO}"* ]]; then
      echo "📂 检测到已有源码，拉取最新代码..."
      cd "$SRC_DIR"
      git fetch --all
      git reset --hard "origin/${BRANCH}"
    else
      echo "⚠️ 检测到已有源码来自其他源 (${current_origin})，将重新克隆..."
      rm -rf "$SRC_DIR"
      git clone --depth 1 -b "$BRANCH" "$clone_url" "$SRC_DIR"
      cd "$SRC_DIR"
    fi
  else
    echo "📥 克隆仓库到 ${SRC_DIR}..."
    rm -rf "$SRC_DIR"
    git clone --depth 1 -b "$BRANCH" "$clone_url" "$SRC_DIR"
    cd "$SRC_DIR"
  fi
  echo "✅ 代码准备完成"
}

# 从源码编译
build_from_source() {
  echo "🔨 开始编译..."

  # 编译 flux_agent (gost)
  echo "🔨 编译 flux_agent..."
  cd "$SRC_DIR/go-gost"
  if ! CGO_ENABLED=0 go build -ldflags="-s -w" -o "$INSTALL_DIR/flux_agent" .; then
    echo "❌ flux_agent 编译失败"
    exit 1
  fi
  if [[ ! -f "$INSTALL_DIR/flux_agent" ]]; then
    echo "❌ flux_agent 二进制文件未生成"
    exit 1
  fi
  chmod +x "$INSTALL_DIR/flux_agent"
  echo "✅ flux_agent 编译完成"

  # 编译 tcp_pool
  if [[ -f "$SRC_DIR/go-gost/tcp-preconn/tcp_pool.c" ]]; then
    if command -v gcc &> /dev/null; then
      echo "🔨 编译 tcp_pool..."
      if gcc -O2 -static -o "$INSTALL_DIR/tcp_pool" "$SRC_DIR/go-gost/tcp-preconn/tcp_pool.c" -lpthread 2>/dev/null; then
        echo "✅ tcp_pool 编译完成（静态链接）"
      elif gcc -O2 -o "$INSTALL_DIR/tcp_pool" "$SRC_DIR/go-gost/tcp-preconn/tcp_pool.c" -lpthread 2>/dev/null; then
        echo "✅ tcp_pool 编译完成（动态链接）"
      else
        echo "⚠️ tcp_pool 编译失败，TCP预连接功能将不可用"
      fi
      if [[ -f "$INSTALL_DIR/tcp_pool" ]]; then
        chmod +x "$INSTALL_DIR/tcp_pool"
      fi
    else
      echo "⚠️ 未安装 gcc，跳过 tcp_pool 编译（TCP预连接功能将不可用）"
    fi
  fi

  echo "✅ 编译全部完成"
}

# 显示菜单
show_menu() {
  echo "==============================================="
  echo "              管理脚本"
  echo "==============================================="
  echo "请选择操作："
  echo "1. 安装"
  echo "2. 更新"  
  echo "3. 卸载"
  echo "4. 退出"
  echo "==============================================="
}

# 删除脚本自身
delete_self() {
  echo ""
  echo "🗑️ 操作已完成，正在清理脚本文件..."
  SCRIPT_PATH="$(readlink -f "$0" 2>/dev/null || realpath "$0" 2>/dev/null || echo "$0")"
  sleep 1
  rm -f "$SCRIPT_PATH" && echo "✅ 脚本文件已删除" || echo "❌ 删除脚本文件失败"
}

# 检查并安装 tcpkill
check_and_install_tcpkill() {
  # 检查 tcpkill 是否已安装
  if command -v tcpkill &> /dev/null; then
    return 0
  fi
  
  # 检测操作系统类型
  OS_TYPE=$(uname -s)
  
  # 检查是否需要 sudo
  if [[ $EUID -ne 0 ]]; then
    SUDO_CMD="sudo"
  else
    SUDO_CMD=""
  fi
  
  if [[ "$OS_TYPE" == "Darwin" ]]; then
    if command -v brew &> /dev/null; then
      brew install dsniff &> /dev/null
    fi
    return 0
  fi
  
  # 检测 Linux 发行版并安装对应的包
  if [ -f /etc/os-release ]; then
    . /etc/os-release
    DISTRO=$ID
  elif [ -f /etc/redhat-release ]; then
    DISTRO="rhel"
  elif [ -f /etc/debian_version ]; then
    DISTRO="debian"
  else
    return 0
  fi
  
  case $DISTRO in
    ubuntu|debian)
      $SUDO_CMD apt update &> /dev/null
      $SUDO_CMD apt install -y dsniff &> /dev/null
      ;;
    centos|rhel|fedora)
      if command -v dnf &> /dev/null; then
        $SUDO_CMD dnf install -y dsniff &> /dev/null
      elif command -v yum &> /dev/null; then
        $SUDO_CMD yum install -y dsniff &> /dev/null
      fi
      ;;
    alpine)
      $SUDO_CMD apk add --no-cache dsniff &> /dev/null
      ;;
    arch|manjaro)
      $SUDO_CMD pacman -S --noconfirm dsniff &> /dev/null
      ;;
    opensuse*|sles)
      $SUDO_CMD zypper install -y dsniff &> /dev/null
      ;;
    gentoo)
      $SUDO_CMD emerge --ask=n net-analyzer/dsniff &> /dev/null
      ;;
    void)
      $SUDO_CMD xbps-install -Sy dsniff &> /dev/null
      ;;
  esac
  
  return 0
}


# 获取用户输入的配置参数
get_config_params() {
  if [[ -z "$SERVER_ADDR" || -z "$SECRET" ]]; then
    echo "请输入配置参数："
    
    if [[ -z "$SERVER_ADDR" ]]; then
      read -p "服务器地址: " SERVER_ADDR
    fi
    
    if [[ -z "$SECRET" ]]; then
      read -p "密钥: " SECRET
    fi
    
    if [[ -z "$SERVER_ADDR" || -z "$SECRET" ]]; then
      echo "❌ 参数不完整，操作取消。"
      exit 1
    fi
  fi
}

# 解析命令行参数
UPDATE_MODE=""
while getopts "a:s:u" opt; do
  case $opt in
    a) SERVER_ADDR="$OPTARG" ;;
    s) SECRET="$OPTARG" ;;
    u) UPDATE_MODE="true" ;;
    *) echo "❌ 无效参数"; exit 1 ;;
  esac
done

# 安装功能
install_flux_agent() {
  echo "🚀 开始安装 flux_agent..."

  ask_proxy_config
  get_config_params

  # 安装编译依赖
  check_git
  if ! ensure_go_installed; then
    echo "❌ Go 安装失败，无法继续安装"
    exit 1
  fi
  ensure_gcc_installed || true
  check_and_install_tcpkill

  mkdir -p "$INSTALL_DIR"

  # 停止并禁用已有服务
  if systemctl list-units --full -all | grep -Fq "flux_agent.service"; then
    echo "🔍 检测到已存在的flux_agent服务"
    systemctl stop flux_agent 2>/dev/null && echo "🛑 停止服务"
    systemctl disable flux_agent 2>/dev/null && echo "🚫 禁用自启"
  fi

  # 删除旧文件
  [[ -f "$INSTALL_DIR/flux_agent" ]] && echo "🧹 删除旧文件 flux_agent" && rm -f "$INSTALL_DIR/flux_agent"

  # 克隆仓库并编译
  clone_or_pull_repo
  build_from_source

  echo "✅ 编译安装完成"

  # 打印版本
  echo "🔎 flux_agent 版本：$($INSTALL_DIR/flux_agent -V)"

  # 写入 config.json (安装时总是创建新的)
  CONFIG_FILE="$INSTALL_DIR/config.json"
  echo "📄 创建新配置: config.json"
  cat > "$CONFIG_FILE" <<EOF
{
  "addr": "$SERVER_ADDR",
  "secret": "$SECRET"
}
EOF

  # 写入 gost.json
  GOST_CONFIG="$INSTALL_DIR/gost.json"
  if [[ -f "$GOST_CONFIG" ]]; then
    echo "⏭️ 跳过配置文件: gost.json (已存在)"
  else
    echo "📄 创建新配置: gost.json"
    cat > "$GOST_CONFIG" <<EOF
{}
EOF
  fi

  # 加强权限
  chmod 600 "$INSTALL_DIR"/*.json

  # 创建 systemd 服务
  SERVICE_FILE="/etc/systemd/system/flux_agent.service"
  cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=Flux_agent Proxy Service
After=network.target

[Service]
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/flux_agent
Restart=always
RestartSec=2
StandardOutput=null
StandardError=null

[Install]
WantedBy=multi-user.target
EOF

  # 启动服务
  systemctl daemon-reload
  systemctl enable flux_agent
  systemctl start flux_agent

  # 检查状态
  echo "🔄 检查服务状态..."
  if systemctl is-active --quiet flux_agent; then
    echo "✅ 安装完成，flux_agent服务已启动并设置为开机启动。"
    echo "📁 配置目录: $INSTALL_DIR"
    echo "📂 源码目录: $SRC_DIR"
    echo "🔧 服务状态: $(systemctl is-active flux_agent)"
  else
    echo "❌ flux_agent服务启动失败，请执行以下命令查看状态："
    echo "systemctl status flux_agent --no-pager"
  fi
}

# 更新功能
update_flux_agent() {
  echo "🔄 开始更新 flux_agent..."
  
  if [[ ! -d "$INSTALL_DIR" ]]; then
    echo "❌ flux_agent 未安装，请先选择安装。"
    return 1
  fi

  ask_proxy_config

  # 安装编译依赖
  check_git
  if ! ensure_go_installed; then
    echo "❌ Go 安装失败，无法继续更新"
    return 1
  fi
  ensure_gcc_installed || true
  check_and_install_tcpkill

  # 拉取最新代码
  clone_or_pull_repo

  # 编译到临时位置
  echo "🔨 编译新版本..."
  cd "$SRC_DIR/go-gost"
  CGO_ENABLED=0 go build -ldflags="-s -w" -o "$INSTALL_DIR/flux_agent.new" .
  if [[ ! -f "$INSTALL_DIR/flux_agent.new" || ! -s "$INSTALL_DIR/flux_agent.new" ]]; then
    echo "❌ 编译 flux_agent 失败"
    return 1
  fi

  # 编译 tcp_pool
  if [[ -f "$SRC_DIR/go-gost/tcp-preconn/tcp_pool.c" ]] && command -v gcc &> /dev/null; then
    gcc -O2 -static -o "$INSTALL_DIR/tcp_pool.new" "$SRC_DIR/go-gost/tcp-preconn/tcp_pool.c" -lpthread 2>/dev/null || \
    gcc -O2 -o "$INSTALL_DIR/tcp_pool.new" "$SRC_DIR/go-gost/tcp-preconn/tcp_pool.c" -lpthread 2>/dev/null || true
  fi

  if systemctl list-units --full -all | grep -Fq "flux_agent.service"; then
    echo "🛑 停止 flux_agent 服务..."
    systemctl stop flux_agent
  fi

  echo "🔄 替换文件..."
  mv "$INSTALL_DIR/flux_agent.new" "$INSTALL_DIR/flux_agent"
  chmod +x "$INSTALL_DIR/flux_agent"

  if [[ -f "$INSTALL_DIR/tcp_pool.new" ]]; then
    mv "$INSTALL_DIR/tcp_pool.new" "$INSTALL_DIR/tcp_pool"
    chmod +x "$INSTALL_DIR/tcp_pool"
  fi
  
  # 打印版本
  echo "🔎 新版本：$($INSTALL_DIR/flux_agent -V)"

  # 重启服务
  echo "🔄 重启服务..."
  systemctl start flux_agent
  
  echo "✅ 更新完成，服务已重新启动。"
}

# 卸载功能
uninstall_flux_agent() {
  echo "🗑️ 开始卸载 flux_agent..."
  
  read -p "确认卸载 flux_agent 吗？此操作将删除所有相关文件 (y/N): " confirm
  if [[ "$confirm" != "y" && "$confirm" != "Y" ]]; then
    echo "❌ 取消卸载"
    return 0
  fi

  # 停止并禁用服务
  if systemctl list-units --full -all | grep -Fq "flux_agent.service"; then
    echo "🛑 停止并禁用服务"
    systemctl stop flux_agent 2>/dev/null
    systemctl disable flux_agent 2>/dev/null
  fi

  if [[ -f "/etc/systemd/system/flux_agent.service" ]]; then
    rm -f "/etc/systemd/system/flux_agent.service"
    echo "🧹 删除服务文件"
  fi

  # 删除安装目录
  if [[ -d "$INSTALL_DIR" ]]; then
    rm -rf "$INSTALL_DIR"
    echo "🧹 删除安装目录: $INSTALL_DIR"
  fi

  # 删除源码目录
  if [[ -d "$SRC_DIR" ]]; then
    rm -rf "$SRC_DIR"
    echo "🧹 删除源码目录: $SRC_DIR"
  fi

  # 重载 systemd
  systemctl daemon-reload

  echo "✅ 卸载完成"
}

# 主逻辑
main() {
  # 如果提供了 -u 参数，直接执行更新
  if [[ "$UPDATE_MODE" == "true" ]]; then
    update_flux_agent
    delete_self
    exit 0
  fi

  # 如果提供了命令行参数，直接执行安装
  if [[ -n "$SERVER_ADDR" && -n "$SECRET" ]]; then
    install_flux_agent
    delete_self
    exit 0
  fi

  # 显示交互式菜单
  while true; do
    show_menu
    read -p "请输入选项 (1-4): " choice
    
    case $choice in
      1)
        install_flux_agent
        delete_self
        exit 0
        ;;
      2)
        update_flux_agent
        delete_self
        exit 0
        ;;
      3)
        uninstall_flux_agent
        delete_self
        exit 0
        ;;
      4)
        echo "👋 退出脚本"
        delete_self
        exit 0
        ;;
      *)
        echo "❌ 无效选项，请输入 1-4"
        echo ""
        ;;
    esac
  done
}

# 执行主函数
main
