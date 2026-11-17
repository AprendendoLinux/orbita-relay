#!/bin/sh
set -e
if [ "$(id -u)" = "0" ]; then
  mkdir -p /data /config
  chown orbita:orbita /data /config
  find /data -maxdepth 1 -type f -exec chown orbita:orbita {} +
  chown -R orbita:orbita /config
  exec su-exec orbita "$@"
fi
exec "$@"
