FROM mongo:8.0-noble AS mongo-tools

FROM golang:1.26-bookworm AS ui-build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/backup-ui ./cmd/backup-ui
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /backup-ui ./cmd/backup-ui

FROM ubuntu:noble

RUN apt-get update \
  && DEBIAN_FRONTEND=noninteractive apt-get install --yes --no-install-recommends ca-certificates cron libgssapi-krb5-2 \
  && rm -rf /var/lib/apt/lists/*

COPY --from=mongo-tools /usr/bin/mongodump /usr/bin/mongorestore /usr/local/bin/
COPY --from=ui-build /backup-ui /usr/local/bin/backup-ui
COPY backup-now list-backups prune-backups /usr/local/bin/

RUN chmod +x /usr/local/bin/backup-now /usr/local/bin/backup-ui /usr/local/bin/list-backups /usr/local/bin/prune-backups \
  && mkdir -p /backups

VOLUME ["/backups"]

CMD ["cron", "-f"]
