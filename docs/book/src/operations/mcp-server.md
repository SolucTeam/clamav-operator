# MCP Server (read-only)

The ClamAV Operator ships a **read-only MCP (Model Context Protocol) server** that lets
AI agents (Claude Code, Claude Desktop, Copilot Workspace, ...) inspect and reason about
your antivirus scans.

It connects to the Kubernetes API with your local kubeconfig (or the in-cluster service
account) and speaks the **stdio** transport, so it can be launched directly by any MCP
client.

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

| Flag | Description |
|---|---|
| `--kubeconfig` | Path to a kubeconfig file (auto-registered by controller-runtime; defaults to `$KUBECONFIG`, in-cluster, then `~/.kube/config`) |
| `--namespace` | Restrict every tool to this namespace (default: all readable namespaces) |
| `--version` | Print version and exit |

## Example prompts

- *"What is the current antivirus status of my cluster?"* → `get_cluster_status`
- *"Which files were infected by the last scan on node-a?"* → `list_infections --nodeName node-a`
- *"Show me the failed scans of the last cluster scan"* → `get_scan --name <cluster-scan>`

## Notes

- All logging goes to **stderr**; stdout is reserved for the MCP protocol.
- `ScanPolicy` is only served in `v1alpha1` (no `v1beta1` version exists); the other
  resources are read through `v1beta1` (the storage/hub version).
- Infected-file lists in `NodeScan.status` are capped by the operator (first 100
  entries); `list_infections` reports `truncated: true` when that limit was hit.
