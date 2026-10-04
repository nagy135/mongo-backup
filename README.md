![MongoDB Backup terminal UI](docs/mongo-backup.png)

Small Docker image for scheduled MongoDB archive backups and interactive restores. It contains MongoDB's `mongodump` and `mongorestore` tools, Debian cron, and a full-screen terminal UI built with Bubble Tea.

## Installation

### Pull from Docker Hub

Add the `backup` service and `mongo_backups` volume to the application's `docker-compose.yml`. Update `MONGODB_URI` to match the MongoDB service name, credentials, and database name used by the application.

```yaml
services:
  backup:
    image: viktornagy/mongo-backup:latest
    restart: unless-stopped
    depends_on:
      - mongo
    environment:
      MONGODB_URI: mongodb://mongo:27017/my-database
      BACKUP_CRON_SCHEDULE: ${BACKUP_CRON_SCHEDULE:-0 2 * * *}
      BACKUP_RETENTION_DAYS: ${BACKUP_RETENTION_DAYS:-7}
      BACKUP_RETENTION_POLICY: ${BACKUP_RETENTION_POLICY:-}
    command:
      - /bin/sh
      - -c
      - |
        cat <<EOF | crontab -
        SHELL=/bin/sh
        PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
        MONGODB_URI="$$MONGODB_URI"
        BACKUP_DIRECTORY="/backups"
        BACKUP_RETENTION_DAYS="$$BACKUP_RETENTION_DAYS"
        BACKUP_RETENTION_POLICY="$$BACKUP_RETENTION_POLICY"
        $$BACKUP_CRON_SCHEDULE backup-now >> /proc/1/fd/1 2>> /proc/1/fd/2
        EOF
        exec cron -f
    volumes:
      - mongo_backups:/backups

volumes:
  mongo_backups:
```

Pull the image and start the service:

```sh
docker compose pull backup
docker compose up -d backup
```

Use a versioned image such as `viktornagy/mongo-backup:v1.0.0` instead of `latest` to keep deployments pinned to a specific release.

### Build from Source

From the directory that contains the application's `docker-compose.yml`, clone this repository:

```sh
git clone https://github.com/nagy135/mongo-backup.git
```

In the Compose configuration above, replace the `image` line with:

```yaml
services:
  backup:
    build:
      context: ./mongo-backup
      dockerfile: Dockerfile
```

Then build and start the service:

```sh
docker compose up -d --build backup
```

`MONGODB_URI` must use the Compose network hostname of the existing MongoDB service (`mongo` in the example), not `localhost`. `localhost` inside the backup container refers to the backup container itself.

## Docker Hub Releases

GitHub Actions publishes version tags whose commits are on `master` to Docker Hub. Each release updates both the versioned image and `latest`.

Configure the GitHub repository secret `DOCKERHUB_TOKEN` with a Docker Hub personal access token that can write to `viktornagy/mongo-backup`. Then create and push a semantic version tag from `master`:

```sh
git switch master
git pull --ff-only
git tag v1.0.0
git push origin v1.0.0
```

This publishes:

```text
viktornagy/mongo-backup:v1.0.0
viktornagy/mongo-backup:latest
```

Tags such as `1.0.0` are also supported. Non-version tags and tags whose commits are not contained in `master` are skipped.

## Configuration

| Variable                  | Default     | Description                                                      |
| ------------------------- | ----------- | ---------------------------------------------------------------- |
| `MONGODB_URI`             | Required    | Full connection URI and database to back up.                     |
| `BACKUP_CRON_SCHEDULE`    | `0 2 * * *` | Standard five-field cron expression, evaluated in UTC.           |
| `BACKUP_RETENTION_DAYS`   | `7`         | Delete archives older than this number of days.                  |
| `BACKUP_RETENTION_POLICY` | Empty       | Optional comma-separated `maximum-age:interval` retention tiers. |
| `BACKUP_DIRECTORY`        | `/backups`  | Archive directory inside the container.                          |

For example, run a backup every six hours and retain it for 14 days:

```dotenv
BACKUP_CRON_SCHEDULE=0 */6 * * *
BACKUP_RETENTION_DAYS=14
```

### Tiered retention

To create a backup every 10 minutes, retain that resolution for the first day,
then retain one backup per hour through day 7 and one per day through day 30:

```dotenv
BACKUP_CRON_SCHEDULE=*/10 * * * *
BACKUP_RETENTION_POLICY=1d:10m,7d:1h,30d:1d
```

The backup job always runs at the cron schedule's frequency. After each successful
backup, the retention policy keeps the newest archive in each 10-minute, hourly,
or daily UTC bucket and removes the others. Archives older than the final tier are
deleted. Supported duration units are `m` (minutes), `h` (hours), and `d` (days).
Tier maximum ages must increase and intervals must stay the same or increase.

`BACKUP_RETENTION_POLICY` takes precedence over `BACKUP_RETENTION_DAYS`. Leave it
empty to keep the original age-only retention behavior.

## Operations

Start or recreate the service:

```sh
docker compose up -d backup
```

Create an archive immediately:

```sh
docker compose exec backup backup-now
```

List archive paths:

```sh
docker compose exec backup list-backups
```

Open the interactive UI:

```sh
docker compose exec backup backup-ui
```

The UI can create backups, list them, rename a backup by adding a `-suffix`, and restore one. A restore runs a non-destructive preflight, then requires confirmation before using `mongorestore --drop` to replace matching collections.

The archive list, selected archive details, actions, and activity log stay on screen.
Each archive shows a compact age marker, such as `20m`, `5h`, or `2d 5h`,
and updates automatically. Backups less than a minute old show `<1m`.
Age uses the UTC timestamp in the backup filename, including renamed archives;
external archives without that timestamp use their file modification time.
Use `Tab` to switch panels, arrow keys or `j`/`k` to navigate, and `Enter` to run
the highlighted action. You can also click panels and archive rows; click an
action again to run it.

| Key | Action |
| --- | --- |
| `b` | Create a backup |
| `r` | Preflight and restore the selected archive |
| `n` | Rename the selected archive with a suffix |
| `p` | Print all backup dates and quit |
| `/` | Filter archives by name |
| `R` | Refresh archives (also refreshes automatically every five seconds) |
| `1` / `2` / `3` | Focus archives / actions / activity |
| `?` | Show keyboard help |
| `Esc` | Clear the filter or cancel a dialog |
| `q` | Quit when idle |
| `Ctrl+C` | Stop the active operation, release its lock, and quit |

Press `p` (or choose **Print dates and quit** in Actions) to close the UI and leave
a plain-text list of every archive's date and time in your terminal, ready to copy
and share. This rereads the backup directory, ignores the current filter, and
prints one UTC timestamp per archive, newest first, including seconds. External
archives without a capture timestamp are explicitly marked as file modification
times. The action is available when idle and does not modify any backups.

To restore an external archive, copy it to the backup volume first:

```sh
docker compose cp ./external.archive.gz backup:/backups/
docker compose exec backup backup-ui
```

### Try the UI with sample data

The test stack starts a separate MongoDB, seeds four collections, and creates an
initial backup. Its database and backup volumes are separate from your application.
Start it from this repository:

```sh
docker compose -p mongo-backup-demo -f docker-compose.test.yml up -d --build --wait
docker compose -p mongo-backup-demo -f docker-compose.test.yml exec backup backup-ui
```

The same stack supports the regular CLI commands:

```sh
docker compose -p mongo-backup-demo -f docker-compose.test.yml exec backup backup-now
docker compose -p mongo-backup-demo -f docker-compose.test.yml exec backup list-backups
```

To try a restore, create a backup, change the sample data, then select the archive
in the UI and press `r`:

```sh
docker compose -p mongo-backup-demo -f docker-compose.test.yml exec mongo \
  mongosh backup-ui-test --quiet --eval 'db.customers.updateOne({_id: 1}, {$set: {name: "Changed after backup"}})'
```

After restoring, this prints `Ada Lovelace` again:

```sh
docker compose -p mongo-backup-demo -f docker-compose.test.yml exec mongo \
  mongosh backup-ui-test --quiet --eval 'db.customers.findOne({_id: 1}).name'
```

Stop the demo while keeping its archives with `docker compose -p mongo-backup-demo
-f docker-compose.test.yml down`. To reset all sample data and archives:

```sh
docker compose -p mongo-backup-demo -f docker-compose.test.yml down -v
```

### Develop the UI locally

Go 1.24 or newer is required. Run `./backup-ui` to launch from source, or build a
native executable with `go build -o bin/backup-ui ./cmd/backup-ui`. Local backup
operations also require the MongoDB tools and executable copies of this
repository's shell commands on `PATH` (`chmod +x backup-now list-backups` in this
checkout); the source launcher adds the repository to `PATH` automatically. The
Docker image includes everything needed and installs the compiled `backup-ui`
binary directly. Run UI and operation tests with `go test ./...`.

## Persisting Archives

`mongo_backups` is a Docker named volume. It survives container recreation but is stored on the Docker host. Back up or export its archive files to off-host storage for disaster recovery.
