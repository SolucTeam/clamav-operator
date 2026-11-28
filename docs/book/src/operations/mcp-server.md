# MCP Server (read-only)

The ClamAV Operator ships a **read-only MCP (Model Context Protocol) server** that lets
AI agents (Claude Code, Claude Desktop, OpenCode, ...) inspect and reason about
your antivirus scans.

Two transports are supported:

| Transport | Use case | Authentication |
|---|---|---|
| `stdio` (default) | Local development: the binary is launched by your MCP client | Ambient kubeconfig / in-cluster credentials |
| `http` | Shared team service deployed in-cluster | Bearer token validated via the Kubernetes **TokenReview** API, per-user RBAC |

In both modes the server is **strictly read-only** (Kubernetes `get`/`list` only) and
**audit-logged**: every tool call produces a structured JSON entry with the caller
identity, arguments, outcome and duration.

## Build

```bash
make build-mcp
# -> bin/clamav-operator-mcp
```

## Tools

| Tool | Description |
|---|---|
| `list_scans` | List `NodeScan`s or `ClusterScan`s, optionally filtered by namespace and phase |
| `get_scan` | Full status of one scan: phase, timing, counters, per-node breakdown, failure reason |
| `list_infections` | Infected files aggregated across scans: node, path, viruses, size, detection time |
| `get_cluster_status` | Operational overview: scans by phase, totals, infected nodes, active schedules |
| `list_schedules` | `ScanSchedule`s: cron expression, suspension state, last/next run |
| `list_policies` | `ScanPolicy`s: paths, exclusions, limits, quarantine and notification config |

The server is **strictly read-only**: it only issues Kubernetes `get`/`list` calls.
Access is governed by the RBAC of the credentials used, so an agent can never see more
than the user behind the kubeconfig.

## Claude Code

```bash
claude mcp add clamav-operator -- /path/to/clamav-operator-mcp
```

Optionally restrict to one namespace:

```bash
claude mcp add clamav-operator -- /path/to/clamav-operator-mcp --namespace clamav-system
```

## Claude Desktop

Add to `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "clamav-operator": {
      "command": "/path/to/clamav-operator-mcp",
      "args": ["--namespace", "clamav-system"]
    }
  }
}
```

## Flags

| Flag | Description | Modes |
|---|---|---|
| `--transport` | `stdio` (default) or `http` | both |
| `--http-addr` | HTTP listen address (default `:8080`) | http |
| `--auth` | `tokenreview` (default) or `none` | http |
| `--namespace` | Restrict every tool to this namespace (default: all readable namespaces) | both |
| `--kubeconfig` | Path to a kubeconfig file (auto-registered by controller-runtime; defaults to `$KUBECONFIG`, in-cluster, then `~/.kube/config`) | both |
| `--version` | Print version and exit | both |

## HTTP mode (in-cluster team service)

### Deploy with Helm

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

### How authentication works

1. The caller sends `Authorization: Bearer <token>` — typically a Kubernetes token
   (`kubectl create token <sa>` or an OIDC token your cluster already trusts).
2. The MCP server validates it via the **TokenReview** API.
3. The request is then executed with a Kubernetes client bound to **that exact
   token**: each caller is restricted by their own RBAC, per request, per cluster.
4. Tokens are never logged or persisted.

```bash
# Generate a short-lived token and call the endpoint
TOKEN=$(kubectl create token alice-viewer --duration=12h)
curl -X POST https://mcp.clamav.example.com/mcp \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}'
```

Unauthenticated requests get `401` before any MCP processing. The ServiceAccount
needs `system:auth-delegator` — a sensitive but standard delegation right, scoped
to this dedicated ServiceAccount.

### Client configuration (OpenCode)

```jsonc
// opencode.jsonc
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

Refresh the token periodically (`kubectl create token --duration=12h` inside a shell
wrapper is the simplest pattern). Note that MCP Streamable HTTP sessions are
stateful (`Mcp-Session-Id`): keep 1 replica or use a sticky-session load balancer.

### Multi-cluster

Run **one MCP server per cluster**, each registered as a separate MCP server in your
client (`clamav-operator-prod`, `clamav-operator-staging`, ...). A single instance
only serves the cluster it runs in (or points at via its kubeconfig).

## Audit log

Every tool call emits one structured JSON line (stdout in HTTP mode for container
log collection, stderr in stdio mode):

```json
{
  "level": "info",
  "ts": "2026-11-28T10:42:17Z",
  "logger": "mcp-audit",
  "msg": "tool_call",
  "user": "system:serviceaccount:security:alice",
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
