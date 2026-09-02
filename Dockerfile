FROM mongo:8.0-noble AS mongo-tools

FROM ghcr.io/charmbracelet/gum:v0.17.0 AS gum

FROM ubuntu:noble

RUN apt-get update \
  && DEBIAN_FRONTEND=noninteractive apt-get install --yes --no-install-recommends ca-certificates cron libgssapi-krb5-2 \
  && rm -rf /var/lib/apt/lists/*

COPY --from=mongo-tools /usr/bin/mongodump /usr/bin/mongorestore /usr/local/bin/
COPY --from=gum /usr/local/bin/gum /usr/local/bin/gum
COPY backup-now backup-ui list-backups prune-backups /usr/local/bin/

RUN chmod +x /usr/local/bin/backup-now /usr/local/bin/backup-ui /usr/local/bin/list-backups /usr/local/bin/prune-backups \
  && mkdir -p /backups

VOLUME ["/backups"]

CMD ["cron", "-f"]
