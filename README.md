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
