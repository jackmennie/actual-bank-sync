# actual-up-sync

A one-shot Go importer for UP transactions into Actual Budget. Go handles scheduling and bank-provider access; a small Node.js helper uses Actual's supported `@actual-app/api` package to read and update the budget. Actual does not expose budget accounts or transactions through a simple REST API.

## Requirements

- Go 1.26 or newer
- Node.js 22 or newer
- An Actual server, its password, and the budget Sync ID
- An UP personal access token

Install the helper dependency once from the project directory:

```bash
npm ci --prefix helper
```

Copy `config.example.yaml` to `config.yaml` and set the Actual server URL, password, Sync ID, and UP API key. `encryptionKey` is only needed for an end-to-end encrypted budget. The helper defaults to `node`, `helper/actual-api.mjs`, and `data/actual`; configure absolute `helperPath` and `dataDir` paths when running from a scheduler or an installed package.

## Build

```bash
make build
```

## Run

```bash
./bin/actual-bank-sync setup
```

The first setup run discovers Actual and UP accounts and prompts for each UP-to-Actual mapping; pressing Enter skips that UP account. It then shows a mapping review before saving to `accounts.yaml` by default. Later setup runs preserve existing mappings and open edit mode: reassign or remove mappings, and add mappings only when new UP accounts are detected. The mapping file is written with private file permissions.

Run one import pass with:

```bash
./bin/actual-bank-sync sync
```

Normal sync prints progress for each mapped account. Add `--verbose` for detailed Actual API diagnostics, or set `verbose: true` in `config.yaml`:

```bash
./bin/actual-bank-sync sync --verbose
```

Preview the reconciliation without importing transactions or advancing sync checkpoints:

```bash
./bin/actual-bank-sync sync --dry-run
```

Write a CSV showing additions and updates, including current Actual values and field-level diffs:

```bash
./bin/actual-bank-sync sync --dry-run --report preview.csv
```

The report contains financial data and is created with owner-only file permissions. `--report` requires `--dry-run`.

The sync command imports settled UP transactions into the mapped Actual accounts. Actual's `importTransactions` reconciliation uses each UP transaction ID as `imported_id` to avoid duplicates. UP transfers are linked to Actual's existing transfer payees when both UP accounts have mappings; transfers to unmapped UP accounts are imported as regular payees. Normal runs store per-account checkpoints in `sync-state.yaml` by default and re-fetch an overlapping seven-day window so delayed settlements are less likely to be missed. Dry runs still connect to Actual and may refresh its local cache, but use Actual's preview reconciliation and do not write imported transactions or sync checkpoints. The first normal run looks back 90 days by default; adjust `initialLookbackDays` in `config.yaml` if needed. A lock file prevents two sync processes from running concurrently.

`accounts.yaml`, `sync-state.yaml`, and their lock files are local runtime data and are ignored by Git by default. Keep these files persistent and backed up alongside the Actual cache if the installation is upgraded or moved.

Keep the helper's npm dependencies installed beside `helper/actual-api.mjs`, and ensure the scheduler can access the configured Actual cache directory and state files. For example, schedule `cd /opt/actual-bank-sync && /usr/local/bin/actual-bank-sync sync`.

## Debian Package

Build the package on Debian or Ubuntu Linux on the target architecture. The build machine needs Go, Node.js 22+, npm, `dpkg-deb`, `dpkg-architecture`, and the toolchain needed by npm's native dependencies.

```bash
make deb
```

On an ARM64 Mac with Docker, build it in a Debian container. The container only needs package-building tools; do not install `cron` or `logrotate` in the builder because they are runtime dependencies of the resulting package:

```bash
docker run --rm --platform linux/arm64 \
	-v "$PWD:/workspace" -w /workspace \
	node:22-bookworm bash -lc '
		set -e
		apt-get update -qq
		apt-get install -y -qq ca-certificates curl dpkg-dev xz-utils build-essential python3
		curl -fsSLo /tmp/go.tar.gz https://go.dev/dl/go1.26.5.linux-arm64.tar.gz
		tar -C /usr/local -xzf /tmp/go.tar.gz
		export PATH="/usr/local/go/bin:$PATH"
		make deb VERSION=0.1.0-1
	'
```

The package is written to `dist/actual-bank-sync_0.1.0-1_arm64.deb`. If cron itself fails to configure on the target system, diagnose that separately from building the package; this build container intentionally does not configure cron.

The package installs the binary, Actual API helper, and its locked production dependencies. It creates the `actual-bank-sync` system user, installs `/etc/actual-bank-sync/config.yaml` from the example on first install, and runs `sync` every 15 minutes from `/etc/cron.d/actual-bank-sync`. Logs go to `/var/log/actual-bank-sync/sync.log`; logrotate retains 14 daily files. Set credentials and the Actual Sync ID in the config file after installation. The cron job runs as the dedicated system user; mappings, checkpoints, and Actual's local cache persist under `/var/lib/actual-bank-sync` and are preserved on package removal.

To select a package version, run `make deb VERSION=0.2.0-1`. To change the cron interval, edit `debian/cron.d` before building the package.
