#!/bin/sh
set -eu

data_dir="${IPAN_DATA_DIR:-/app/data}"

if [ "$(id -u)" = "0" ]; then
  if ! mkdir -p "$data_dir"; then
    echo "[ipan] 无法创建数据目录: $data_dir" >&2
    exit 1
  fi

  # Existing named volumes and NAS bind mounts are often created by root.
  # Repair ownership before dropping privileges.
  chown -R ipan:ipan "$data_dir" 2>/dev/null || true
  if ! su-exec ipan:ipan test -w "$data_dir"; then
    echo "[ipan] 数据目录不可写: $data_dir" >&2
    echo "[ipan] 请确认挂载目录允许容器用户 ipan（UID 100 / GID 101）写入。" >&2
    exit 1
  fi

  exec su-exec ipan:ipan /app/ipan "$@"
fi

exec /app/ipan "$@"
