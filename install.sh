#!/bin/bash
set -e

# GitHub 仓库地址
REPO="Xeloan/flvx-tcppreconn"
BRANCH="main"

# 安装目录
INSTALL_DIR="/etc/flux_agent"

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
      echo "? 参数不完整，操作取消。"
      exit 1
    fi
  fi
}

check_docker() {
  if ! command -v docker &> /dev/null; then
    echo "错误：未检测到 docker 命令。请先安装 Docker（例如: curl -fsSL https://get.docker.com | bash）。"
    exit 1
  fi
}

# 解析命令行参数
UPDATE_MODE=""
while getopts "a:s:u" opt; do
  case $opt in
    a) SERVER_ADDR="$OPTARG" ;;
    s) SECRET="$OPTARG" ;;
    u) UPDATE_MODE="true" ;;
    *) echo "? 无效参数"; exit 1 ;;
  esac
done

cleanup_local_legacy() {
  if systemctl list-units --full -all | grep -Fq "flux_agent.service" 2>/dev/null; then
    echo "?? 检测到旧版通过 systemd 安装的本地 flux_agent，正在停止并清理..."
    systemctl stop flux_agent 2>/dev/null || true
    systemctl disable flux_agent 2>/dev/null || true
    rm -f /etc/systemd/system/flux_agent.service
    systemctl daemon-reload 2>/dev/null || true
  fi
  [[ -f "$INSTALL_DIR/flux_agent" ]] && rm -f "$INSTALL_DIR/flux_agent"
  [[ -f "$INSTALL_DIR/tcp_pool" ]] && rm -f "$INSTALL_DIR/tcp_pool"
}

run_docker_agent() {
  # 停止或清理已有同名容器
  docker rm -f flux_agent >/dev/null 2>&1 || true

  echo "?? 拉取最新 Agent 镜像..."
  # 转换为全小写适配 GHCR
  local repo_lower=$(echo "$REPO" | tr '[:upper:]' '[:lower:]')
  docker pull ghcr.io/${repo_lower}-agent:main

  echo "?? 根据配置启动 Agent 容器..."
  docker run -d \
    --name flux_agent \
    --restart unless-stopped \
    --network host \
    --cap-add=NET_ADMIN \
    --cap-add=NET_RAW \
    -v "$INSTALL_DIR:/etc/flux_agent" \
    ghcr.io/${repo_lower}-agent:main

  echo "?? 容器状态: $(docker ps -f name=flux_agent --format '{{.Status}}')"
}

install_flux_agent() {
  echo "?? 开始基于 Docker 安装 flux_agent..."
  check_docker
  
  if [[ -z "$SERVER_ADDR" || -z "$SECRET" ]]; then
    get_config_params
  fi

  mkdir -p "$INSTALL_DIR"
  cleanup_local_legacy

  CONFIG_FILE="$INSTALL_DIR/config.json"
  echo "?? 创建配置: config.json"
  cat > "$CONFIG_FILE" <<EOF
{
  "addr": "$SERVER_ADDR",
  "secret": "$SECRET"
}
EOF

  GOST_CONFIG="$INSTALL_DIR/gost.json"
  if [[ ! -f "$GOST_CONFIG" ]]; then
    echo "?? 创建空配置: gost.json"
    echo "{}" > "$GOST_CONFIG"
  fi

  chmod 600 "$INSTALL_DIR"/*.json

  run_docker_agent
  echo "? 安装完成，flux_agent 已在 Docker 中运行并设置为开机自启。"
  echo "?? 配置文件挂载于: $INSTALL_DIR"
}

update_flux_agent() {
  echo "?? 开始更新 flux_agent 容器..."
  check_docker

  if [[ ! -d "$INSTALL_DIR" ]]; then
    echo "? 配置文件目录 ($INSTALL_DIR) 不存在，请先安装。"
    return 1
  fi

  cleanup_local_legacy
  run_docker_agent
  echo "? 更新完成"
}

uninstall_flux_agent() {
  echo "??? 开始卸载 flux_agent..."
  
  read -p "确认卸载 flux_agent 吗？此操作将删除所有配置及容器 (y/N): " confirm
  if [[ "$confirm" != "y" && "$confirm" != "Y" ]]; then
    echo "? 取消卸载"
    return 0
  fi

  check_docker
  docker rm -f flux_agent >/dev/null 2>&1 || true
  
  local repo_lower=$(echo "$REPO" | tr '[:upper:]' '[:lower:]')
  docker rmi ghcr.io/${repo_lower}-agent:main >/dev/null 2>&1 || true

  cleanup_local_legacy

  rm -rf "$INSTALL_DIR"
  echo "? 卸载完成，所有文件和容器已被清理。"
}

delete_self() {
  echo ""
  echo "??? 操作已完成，正在清理脚本文件..."
  SCRIPT_PATH="$(readlink -f "$0" 2>/dev/null || realpath "$0" 2>/dev/null || echo "$0")"
  sleep 1
  rm -f "$SCRIPT_PATH" && echo "? 脚本文件已被安全删除" || true
}

show_menu() {
  echo "==============================================="
  echo "           Agent Docker 部署脚本"
  echo "==============================================="
  echo "请选择功能："
  echo "1. 安装"
  echo "2. 更新"  
  echo "3. 卸载"
  echo "4. 退出"
  echo "==============================================="
}

# 若传了命令行参数 -u 则直接更新
if [[ "$UPDATE_MODE" == "true" ]]; then
  update_flux_agent
  delete_self
  exit 0
fi

# 参数全全，直接安装无交互
if [[ -n "$SERVER_ADDR" && -n "$SECRET" ]]; then
  install_flux_agent
  delete_self
  exit 0
fi

# 无参数交互式菜单
show_menu
read -p "请输入对应的数字: " choice
case $choice in
  1) install_flux_agent ;;
  2) update_flux_agent ;;
  3) uninstall_flux_agent ;;
  4) exit 0 ;;
  *) echo "? 无效的选择，请重新运行脚本"; exit 1 ;;
esac

delete_self
