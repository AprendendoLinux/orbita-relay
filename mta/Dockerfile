FROM debian:bookworm-slim

RUN apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      exim4-daemon-light logrotate openssl ca-certificates tzdata \
 && rm -rf /var/lib/apt/lists/*

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 25 587
HEALTHCHECK --interval=30s --timeout=5s --start-period=40s CMD bash -c 'echo > /dev/tcp/127.0.0.1/25' || exit 1
ENTRYPOINT ["/entrypoint.sh"]
