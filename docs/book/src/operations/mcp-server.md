# MCP Server (read-only)

The ClamAV Operator ships a **read-only MCP (Model Context Protocol) server** that lets
AI agents (Claude Code, Claude Desktop, OpenCode, ...) inspect and reason about
your antivirus scans.

The server is deployed **in-cluster as a shared team service** (HTTP transport).
AI agents connect to it over the network with a Kubernetes token: no local build,
no local kubeconfig for the agent, and updates are rolled out by GitOps like the
rest of the operator.

| Transport | Use case | Authentication |
|---|---|---|
| `http` — **standard mode** | Shared team service deployed in-cluster; how every user connects from their workstation | Bearer token validated via the Kubernetes **TokenReview** API, per-user RBAC |
| `stdio` | Development only: run the binary locally against your kubeconfig | Ambient kubeconfig |

In both modes the server is **strictly read-only** (Kubernetes `get`/`list` only) and
**audit-logged**: every tool call produces a structured JSON entry with the caller
identity, arguments, outcome and duration.

## Tools

| Tool | Description |
|---|---|
| `list_scans` | List `NodeScan`s or `ClusterScan`s, optionally filtered by namespace and phase |
| `get_scan` | Full status of one scan: phase, timing, counters, per-node breakdown, failure reason |
| `list_infections` | Infected files aggregated across scans: node, path, viruses, size, detection time |
| `get_cluster_status` | Operational overview: scans by phase, totals, infected nodes, active schedules |
| `list_schedules` | `ScanSchedule`s: cron expression, suspension state, last/next run |
| `list_policies` | `ScanPolicy`s: paths, exclusions, limits, quarantine and notification config |

Access is governed by the RBAC of **the caller's own token**: an agent can never see
more than the user behind the request.

## Deploy with Helm (ops)

```yaml
# values-mcp.yaml
mcpServer:
  enabled: true
  ingress:
    enabled: true
    host: mcp.clamav.example.com
```

```bash
helm upgrade --install clamav-operator ./helm/clamav-operator \
  -f values-mcp.yaml -n clamav-system
```

This creates a dedicated Deployment + Service, a read-only `Role` on the four CRDs
(`nodescans`, `clusterscans`, `scanschedules`, `scanpolicies`), the
`system:auth-delegator` ClusterRoleBinding (needed for TokenReview), and an optional
Ingress with cert-manager TLS.

## Onboard a user (admin)

Each user gets a dedicated ServiceAccount bound to a read-only role:

```yaml
# mcp-user-alice.yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: mcp-alice
  namespace: clamav-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: mcp-alice-reader
  namespace: clamav-system
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: clamav-operator-mcp-reader   # the read-only Role created by the chart
subjects:
- kind: ServiceAccount
  name: mcp-alice
  namespace: clamav-system
```

> Adjust the Role name to the release name (`<release>-mcp-reader`).
> To scope a user to a single namespace, create the RoleBinding in that namespace
> instead, or set `mcpServer.namespace` on a dedicated MCP instance.

## Connect an AI agent (user)

### 1. Get a token

See [Token lifetime & renewal](#token-lifetime--renewal) below. Quick start:

```bash
export CLAMAV_MCP_TOKEN=$(kubectl create token mcp-alice -n clamav-system --duration=12h)
```

### 2. Configure the client

**OpenCode** (`opencode.jsonc`):

```jsonc
{
  "mcp": {
    "servers": {
      "clamav-operator": {
        "type": "remote",
        "url": "https://mcp.clamav.example.com/mcp",
        "oauth": false,
        "headers": {
          "Authorization": "Bearer {env:CLAMAV_MCP_TOKEN}"
        }
      }
    }
  }
}
```

**Claude Code**:

```bash
claude mcp add clamav-operator \
  --transport http --url https://mcp.clamav.example.com/mcp \
  --header "Authorization: Bearer $CLAMAV_MCP_TOKEN"
```

**Claude Desktop / other clients**: add a remote MCP server pointing at the same URL
with the same `Authorization` header.

### How authentication works

1. The caller sends `Authorization: Bearer <token>`.
2. The MCP server validates it via the **TokenReview** API.
3. The request is then executed with a Kubernetes client bound to **that exact
   token**: each caller is restricted by their own RBAC, per request, per cluster.
4. Tokens are never logged or persisted.

Unauthenticated requests get `401` before any MCP processing. The MCP ServiceAccount
needs `system:auth-delegator` — a sensitive but standard delegation right, scoped
to this dedicated ServiceAccount.

## Token lifetime & renewal

Kubernetes tokens are short-lived by design. Two supported options:

### Option 1 — Long-lived token (documented shortcut)

Create a token Secret without expiry for the user's ServiceAccount:

```bash
kubectl -n clamav-system create secret generic mcp-alice-token \
  --type kubernetes.io/service-account-token \
  --from-literal=token="" \
  $(kubectl -n clamav-system get sa mcp-alice -o name | sed 's/serviceaccount/service-account/')
# K8s populates the "token" key in the Secret; retrieve it:
kubectl -n clamav-system get secret mcp-alice-token -o jsonpath='{.data.token}' | base64 -d
```

- Zero renewal friction; rotation is manual (delete and re-create the Secret, then
  distribute the new token).
- Revocation = delete the Secret.
- Requires `kubernetes.io/service-account-token` secrets to be enabled for the
  ServiceAccount (legacy token generation enabled cluster-wide or per-SA via the
  `kubernetes.io/enforce-mountable-secrets` equivalent — see the Kubernetes docs for
  your version).

### Option 2 — Wrapper shell (fresh token on every launch)

```sh
# ~/.zshrc
opencode() {
  export CLAMAV_MCP_TOKEN=$(kubectl create token mcp-alice -n clamav-system --duration=12h)
  command opencode "$@"
}
```

Every launch starts with a fresh 12 h token — nothing to remember, nothing stored on
disk beyond the shell environment. Same pattern for any MCP client.

### Notes

- MCP Streamable HTTP sessions are stateful (`Mcp-Session-Id`): keep 1 replica or use
  a sticky-session load balancer. Token renewal does not break active sessions.

### Multi-cluster

Run **one MCP server per cluster**, each registered as a separate MCP server in your
client (`clamav-operator-prod`, `clamav-operator-staging`, ...) with its own URL and
token. A single instance only serves the cluster it runs in.

## Flags

| Flag | Description | Modes |
|---|---|---|
| `--transport` | `http` (deployment) or `stdio` (development) | both |
| `--http-addr` | HTTP listen address (default `:8080`) | http |
| `--auth` | `tokenreview` (default) or `none` | http |
| `--namespace` | Restrict every tool to this namespace (default: all readable namespaces) | both |
| `--kubeconfig` | Path to a kubeconfig file (auto-registered by controller-runtime; defaults to `$KUBECONFIG`, in-cluster, then `~/.kube/config`) | both |
| `--version` | Print version and exit | both |

## Development mode (stdio)

For local development against your own kubeconfig — operator contributors only:

```bash
make build-mcp
# -> bin/clamav-operator-mcp
claude mcp add clamav-operator -- /path/to/bin/clamav-operator-mcp
```

In this mode the agent runs with your kubeconfig credentials (renewal handled by the
kubeconfig itself, e.g. OIDC/exec plugins). It is not the team distribution channel.

## Audit log

Every tool call emits one structured JSON line (stdout in HTTP mode for container
log collection, stderr in stdio mode):

```json
{
  "level": "info",
  "ts": "2026-11-28T10:42:17Z",
  "logger": "mcp-audit",
  "msg": "tool_call",
  "user": "system:serviceaccount:security:mcp-alice",
  "tool": "list_infections",
  "arguments": {"nodeName": "node-a"},
  "result": "ok",
  "count": 3,
  "durationMs": 84
}
```

- `user` is the TokenReview-authenticated principal — answer *who* listed *what*,
  *when*.
- No secrets are ever logged: tokens and headers never reach the audit layer.
- `ScanPolicy` is only served in `v1alpha1` (no `v1beta1` version exists); the other
  resources are read through `v1beta1` (the storage/hub version).

## Example prompts

- *"What is the current antivirus status of my cluster?"* → `get_cluster_status`
- *"Which files were infected by the last scan on node-a?"* → `list_infections --nodeName node-a`
- *"Show me the failed scans of the last cluster scan"* → `get_scan --name <cluster-scan>`

## Notes

- In **stdio** mode all logging goes to **stderr**; stdout is reserved for the MCP
  protocol. In **http** mode logs go to **stdout** for container log collection.
- Infected-file lists in `NodeScan.status` are capped by the operator (first 100
  entries); `list_infections` reports `truncated: true` when that limit was hit.
