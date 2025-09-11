#!/bin/bash
# Correct Docker run command for Unraid

docker run \
  -d \
  --name='mcp-memory-rewrite' \
  --net='bridge' \
  --pids-limit 2048 \
  -e TZ="America/New_York" \
  -e HOST_OS="Unraid" \
  -e HOST_HOSTNAME="Server" \
  -e HOST_CONTAINERNAME="mcp-memory-rewrite" \
  -e 'LOG_LEVEL'='INFO' \
  -l net.unraid.docker.managed=dockerman \
  -p '1818:8080/tcp' \
  -v '/mnt/user/appdata/mcp-memory-rewrite/':'/data':'rw' \
  'ghcr.io/jamesprial/mcp-memory-rewrite:latest' \
  -http :8080 -sse