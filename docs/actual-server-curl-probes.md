# Probe the Actual server with curl

These read-only probes help verify the Actual server URL, password login, session token, and available budget files. They do not change server or budget data.

The commands require `curl` and `jq`. Set `ACTUAL_BASE_URL` to the server's base URL, without a trailing slash. For example:

```sh
export ACTUAL_BASE_URL="https://your-actual-server.example"
```

If the server uses a private or self-signed certificate, prefer supplying its CA with curl's `--cacert /path/to/ca.pem`. Avoid `-k` except for a temporary local diagnostic because it disables certificate verification.

## Check the server

```sh
curl --fail-with-body --silent --show-error \
  --connect-timeout 5 --max-time 20 \
  "$ACTUAL_BASE_URL/info" | jq .
```
```json
{
  "build": {
    "name": "@actual-app/sync-server",
    "description": "actual syncing server",
    "version": "26.8.0"
  }
}
```
```sh
curl --fail-with-body --silent --show-error \
  --connect-timeout 5 --max-time 20 \
  "$ACTUAL_BASE_URL/account/needs-bootstrap" | jq .
```
```json
{
  "status": "ok",
  "data": {
    "bootstrapped": true,
    "loginMethod": "password",
    "availableLoginMethods": [
      {
        "method": "password",
        "active": 1,
        "displayName": "Password"
      }
    ],
    "multiuser": false
  }
}
```
```sh
curl --fail-with-body --silent --show-error \
  --connect-timeout 5 --max-time 20 \
  "$ACTUAL_BASE_URL/account/login-methods" | jq .
```

```json
{
  "status": "ok",
  "methods": [
    {
      "method": "password",
      "active": 1,
      "displayName": "Password"
    }
  ]
}
```

`/account/needs-bootstrap` reports whether the server has been initialized. `/account/login-methods` reports whether password login is available. Do not attempt a login if the server is not bootstrapped.


## Log in and list budget files

Run these commands in the same Bash or Zsh session. The password is read without echo and is not placed directly in the curl command. Keep the returned session token private; it grants access to the server until it expires or is revoked.

```sh
read -r -s "ACTUAL_PASSWORD?Actual password: "
printf '\n'
LOGIN_RESPONSE=$(jq -n --arg password "$ACTUAL_PASSWORD" \
  '{loginMethod: "password", password: $password}' | \
  curl --fail-with-body --silent --show-error \
    --connect-timeout 5 --max-time 20 \
    -H 'Content-Type: application/json' \
    -H 'Accept: application/json' \
    --data-binary @- \
    "$ACTUAL_BASE_URL/account/login")
unset ACTUAL_PASSWORD

printf '%s\n' "$LOGIN_RESPONSE" | jq '{status, reason, hasToken: (.data.token != null)}'
ACTUAL_TOKEN=$(printf '%s' "$LOGIN_RESPONSE" | jq -er '.data.token')
unset LOGIN_RESPONSE

curl --fail-with-body --silent --show-error \
  --connect-timeout 5 --max-time 20 \
  -H "X-ACTUAL-TOKEN: $ACTUAL_TOKEN" \
  -H 'Accept: application/json' \
  "$ACTUAL_BASE_URL/sync/list-user-files" | jq .
```

A successful login returns a token under `.data.token`. In this deployment, sync-server routes are exposed beneath `/sync/`: `/sync/list-user-files` returned JSON with `status: "ok"` and a list of budget files. The same route without the `/sync/` prefix returned HTML, so use the prefixed route for this server.

Check the status and response headers without printing the response body:

```sh
curl --silent --show-error \
  --connect-timeout 5 --max-time 20 \
  -D - -o /dev/null \
  -H "X-ACTUAL-TOKEN: $ACTUAL_TOKEN" \
  -H 'Accept: application/json' \
  -w '\nHTTP %{http_code}; content-type: %{content_type}\n' \
  "$ACTUAL_BASE_URL/sync/list-user-files"
```

This header probe is useful if the route stops returning JSON or you test another deployment.

To inspect metadata for a file, set its ID from the file list and run:

```sh
export ACTUAL_FILE_ID="replace-with-file-id"
curl --fail-with-body --silent --show-error \
  --connect-timeout 5 --max-time 20 \
  -H "X-ACTUAL-TOKEN: $ACTUAL_TOKEN" \
  -H "X-Actual-File-Id: $ACTUAL_FILE_ID" \
  -H 'Accept: application/json' \
  "$ACTUAL_BASE_URL/sync/get-user-file-info" | jq .
```

When finished, clear the shell variables:

```sh
unset ACTUAL_TOKEN ACTUAL_FILE_ID
```

## Endpoint Checklist

This is the inventory of relevant endpoints identified so far. “Yes” means the endpoint was observed working through the configured public server URL; “No” means it is not currently usable or has not yet been verified. The sync-server is reverse-proxied under `/sync/` in this deployment. This list is scoped to the endpoints investigated so far, not every route Actual exposes.

| Endpoint | What it does | Can use (yes/no) | Reason for no |
| --- | --- | --- | --- |
| `GET /info` | Reports server build/version information. | Yes | Not applicable. Verified against the configured server. |
| `GET /account/needs-bootstrap` | Reports whether the server is initialized and its configured login method. | Yes | Not applicable. Verified against the configured server. |
| `GET /account/login-methods` | Lists available authentication methods. | Yes | Not applicable. Verified against the configured server. |
| `POST /account/login` | Authenticates with the server password and returns a session token. | Yes | Not applicable. Password login succeeded and returned a token. |
| `GET /sync/list-user-files` | Lists budget files available to the authenticated user. | Yes | Not applicable. Returned JSON with `status: "ok"` and file data. The unprefixed URL returned HTML. |
| `GET /sync/get-user-file-info` | Returns metadata for a budget file; requires `X-ACTUAL-TOKEN` and `X-Actual-File-Id`. | No | Not yet tested. Uses the observed `/sync/` proxy prefix; verify response before relying on it. |
| `GET /accounts` | Would list accounts inside a budget if a simple endpoint existed. | No | Actual does not provide this as a simple REST endpoint. Accounts are inside the budget data. |
| `POST /sync` | Exchanges budget synchronization data with Actual. | No | This is Actual's specialized binary/CRDT sync protocol, not a simple REST API for querying accounts or importing individual transactions. |

The discovery, authentication, and file-list endpoints are now verified through this deployment, but they do not provide budget account or transaction data. The file metadata route still needs verification. Reading accounts and importing transactions requires Actual's budget sync protocol or its supported application API.

If login returns an authentication error, check `/account/login-methods` first. Repeated invalid password attempts are rate-limited, so avoid retry loops.
