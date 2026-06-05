# Zabbix Agent 2 Docker Swarm Plugin

A production-ready loadable plugin for Zabbix Agent 2 that provides comprehensive monitoring of Docker Swarm services, nodes, and container resource usage.

## Overview

The standard Docker plugin in Zabbix monitors at the container level, which breaks historical data continuity in Swarm because containers get new random IDs on every restart or redeploy. This plugin monitors at the **service level** instead, providing stable identifiers that survive restarts, rolling updates, and full stack redeploys.

## Features

- **Service Discovery**: LLD for all Docker Swarm services with stable `{#SERVICE.KEY}` identifiers
- **Stack Discovery**: LLD for Docker Compose stacks grouped by `com.docker.stack.namespace`
- **Replica Monitoring**: Desired vs running replica counts with placement constraint awareness
- **Restart Detection**: Counts only genuinely failed (crashed) tasks — not rolling updates or scale-downs
- **Stack Health**: Aggregate health percentage per stack with unevaluated-service tracking
- **Node Monitoring**: LLD for all swarm nodes with availability, state and role per node
- **Container Stats**: Per-replica CPU and memory metrics collected locally on each node
- **Stable History**: Service- and slot-based identifiers mean Zabbix item history is never fragmented
- **Cross-Architecture**: Supports x86_64 and ARM64 Linux

## Requirements

- **Zabbix Agent 2**: Version 6.0 or later (template requires 7.4 for embedded dashboard)
- **Go**: Version 1.21+ (for building from source)
- **Docker**: Swarm mode enabled, API version v1.47+
- **Permissions**: Zabbix user must have read access to `/var/run/docker.sock`

## Installation

### One-line install (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/AdureIO/zabbix-agent2-plugin-docker-swarm/main/scripts/install.sh | sudo bash
```

The script auto-detects architecture (x86_64/arm64), downloads the latest release,
verifies the SHA-256 checksum, writes the plugin config to
`/etc/zabbix/zabbix_agent2.d/docker-swarm.conf`, adds the `zabbix` user to the
`docker` group, and restarts `zabbix-agent2`.

**Install a specific version:**
```bash
curl -fsSL .../install.sh | sudo VERSION=v1.0.6 bash
```

**Dry run (preview actions without executing):**
```bash
curl -fsSL .../install.sh | sudo DRY_RUN=1 bash
```

**Uninstall:**
```bash
curl -fsSL .../install.sh | sudo UNINSTALL=1 bash
```

**Available environment variables:**

| Variable | Default | Description |
|---|---|---|
| `VERSION` | latest | Release tag to install (e.g. `v1.0.6`) |
| `INSTALL_DIR` | `/var/lib/zabbix/plugins` | Directory for the plugin binary |
| `CONF_DIR` | `/etc/zabbix/zabbix_agent2.d` | Directory for the plugin config file |
| `SOCKET` | `/var/run/docker.sock` | Docker socket path |
| `NO_RESTART` | `0` | Set to `1` to skip restarting zabbix-agent2 |
| `DRY_RUN` | `0` | Set to `1` to preview actions without executing |
| `UNINSTALL` | `0` | Set to `1` to remove the plugin |

### Manual installation

<details>
<summary>Expand for manual steps</summary>

**1. Download binary**

```bash
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
VERSION=$(curl -sf https://api.github.com/repos/AdureIO/zabbix-agent2-plugin-docker-swarm/releases/latest | grep tag_name | cut -d'"' -f4)
curl -fsSL -o /tmp/docker-swarm \
  "https://github.com/AdureIO/zabbix-agent2-plugin-docker-swarm/releases/download/${VERSION}/docker-swarm-linux-${ARCH}"
```

**2. Install binary**

```bash
sudo install -m 755 -o root -g root /tmp/docker-swarm /var/lib/zabbix/plugins/docker-swarm
```

**3. Write plugin config**

```bash
sudo tee /etc/zabbix/zabbix_agent2.d/docker-swarm.conf <<EOF
Plugins.DockerSwarm.System.Path=/var/lib/zabbix/plugins/docker-swarm
Plugins.DockerSwarm.System.Timeout=30
EOF
```

**4. Grant Docker socket access**

```bash
sudo usermod -aG docker zabbix
```

**5. Restart agent**

```bash
sudo systemctl restart zabbix-agent2
```

</details>

**Build from source:**

```bash
git clone https://github.com/AdureIO/zabbix-agent2-plugin-docker-swarm.git
cd zabbix-agent2-plugin-docker-swarm/src
make build-x86_64   # or make build-arm64
```

### Import the Zabbix Template

Import `zabbix_template_docker_swarm.yaml` from the repository root in Zabbix under
**Configuration → Templates → Import**. The template targets Zabbix 7.4 and includes
an embedded dashboard with three pages (Overview, Services, Resources).

## Multi-Node Deployment

**Node metrics and replica stats require the plugin on every swarm node.**

- `swarm.nodes.discovery` and `swarm.node.status` only need to run on a manager node.
- `swarm.replicas.discovery` and `swarm.replica.stats` each query the **local** Docker socket and only report replicas running on that specific node. Deploy the plugin on every node for full cluster resource visibility.

Zabbix aggregates replica metrics across all nodes automatically via the LLD discovery rules — each node's agent contributes its local replica items to the same host or host group.

## Quick Start

```bash
# Service and stack discovery
zabbix_get -s localhost -k "swarm.services.discovery"
zabbix_get -s localhost -k "swarm.stacks.discovery"

# Stack health
zabbix_get -s localhost -k "swarm.stack.health[mystack]"

# Service replica counts (use service name, ID, or service key)
zabbix_get -s localhost -k "swarm.service.replicas_desired[mystack_web]"
zabbix_get -s localhost -k "swarm.service.replicas_running[mystack_web]"

# Failed task count and restart detection
zabbix_get -s localhost -k "swarm.service.restarts[mystack_web]"
zabbix_get -s localhost -k "swarm.service.last_restart[mystack_web]"

# Node discovery and status (run on manager)
zabbix_get -s localhost -k "swarm.nodes.discovery"
zabbix_get -s localhost -k "swarm.node.status[worker-01]"

# Replica discovery and stats (run on each node)
zabbix_get -s localhost -k "swarm.replicas.discovery"
zabbix_get -s localhost -k "swarm.replica.stats[mystack_web/slot/1]"
```

## Supported Metrics

### Service metrics

| Key | Description | Returns |
|-----|-------------|---------|
| `swarm.services.discovery` | LLD for all services | JSON array with `{#SERVICE.ID}`, `{#SERVICE.NAME}`, `{#STACK.NAME}`, `{#SERVICE.KEY}` |
| `swarm.service.replicas_desired[<id>]` | Desired replica count | Integer |
| `swarm.service.replicas_running[<id>]` | Running task count | Integer |
| `swarm.service.restarts[<id>]` | Failed task count in recent history | Integer |
| `swarm.service.tasks[<id>]` | Total task count (debug) | Integer |
| `swarm.service.last_restart[<id>]` | Unix timestamp of most recent running task start | Integer (unixtime) |

### Stack metrics

| Key | Description | Returns |
|-----|-------------|---------|
| `swarm.stacks.discovery` | LLD for all stacks | JSON array with `{#STACK.NAME}` |
| `swarm.stack.health[<stack>]` | Stack health summary | JSON (see below) |

`swarm.stack.health` JSON fields:

```json
{
  "total_services": 5,
  "evaluated_services": 5,
  "healthy_services": 4,
  "unhealthy_services": 1,
  "unevaluated_services": 0,
  "health_percentage": 80.0
}
```

### Node metrics

| Key | Description | Returns |
|-----|-------------|---------|
| `swarm.nodes.discovery` | LLD for all swarm nodes | JSON array with `{#NODE.ID}`, `{#NODE.HOSTNAME}`, `{#NODE.ROLE}` |
| `swarm.node.status[<id_or_hostname>]` | Node status | JSON (see below) |

`swarm.node.status` JSON fields:

```json
{
  "hostname": "worker-01",
  "role": "worker",
  "availability": "active",
  "state": "ready",
  "addr": "192.168.1.10"
}
```

A healthy node has `availability: active` and `state: ready`.

### Replica metrics (per-node)

| Key | Description | Returns |
|-----|-------------|---------|
| `swarm.replicas.discovery` | LLD for replicas on this node | JSON array with `{#SERVICE.KEY}`, `{#SERVICE.NAME}`, `{#STACK.NAME}`, `{#REPLICA.KEY}`, `{#REPLICA.SLOT}` |
| `swarm.replica.stats[<replica_key>]` | CPU and memory stats | JSON (see below) |

`swarm.replica.stats` JSON fields:

```json
{
  "cpu_percent": 12.5,
  "cpu_ns":      1234567890,
  "mem_bytes":   67108864,
  "mem_percent": 3.2,
  "mem_limit":   2147483648
}
```

- `cpu_percent`: point-in-time CPU percentage (calculated from Docker's built-in pre/post sample pair)
- `cpu_ns`: cumulative CPU nanoseconds — use Zabbix Delta storage type for rate
- `mem_bytes`: working set memory (page cache subtracted), matches `docker stats` output
- `mem_percent`: 0 when no memory limit is configured on the container

Use `swarm.replica.stats` as a **master item** and create dependent items for each field using JSONPath preprocessing. This results in a single Docker API call per replica per interval.

### Replica key format

Replica keys are stable across container restarts:

| Service mode | Key format | Example |
|---|---|---|
| Replicated | `{service_key}/slot/{N}` | `mystack_web/slot/1` |
| Global | `{service_key}/node/{nodeID[:12]}` | `mystack_agent/node/a1b2c3d4ef12` |

For replicated services the slot number never changes — when a container crashes and is replaced, the new task gets the same slot, so Zabbix item history remains continuous.

### Service identifiers

All service metrics accept:

- **Service ID** — full Docker service ID (`abc123def456...`)
- **Service name** — plain name (`web`)
- **Service key** — stable stack-prefixed name (`mystack_web`)

## Desired Replica Calculation

The plugin calculates desired replicas correctly for all service configurations:

- **Replicated services**: uses the configured replica count, capped by `MaxReplicasPerNode × eligible_nodes` when `MaxReplicasPerNode` is set
- **Global services**: counts only nodes that are `availability=active` and `state=ready` and satisfy the service's placement constraints
- **Placement constraints** (`node.role`, `node.labels.*`, `engine.labels.*`, `node.platform.*`, etc.) are fully evaluated for both global and replicated services

This prevents false replica-mismatch alerts when nodes are drained, paused, or constrained by placement rules.

## Restart Detection

`swarm.service.restarts` counts only tasks with `state=failed` (container crashed). It deliberately excludes:

- Tasks in transitional states (`preparing`, `starting`, `assigned`) — normal container lifecycle
- Tasks intentionally stopped (`state=shutdown`) — rolling updates and scale-downs

Because Docker retains only the last ~5 tasks per slot, this counter is **not cumulative** — do not use Delta storage type. Use `change()>0` in triggers instead:

```
# Detect new task failures
change(/YourHost/swarm.service.restarts[{#SERVICE.KEY}])>0

# Detect any restart (more sensitive)
change(/YourHost/swarm.service.last_restart[{#SERVICE.KEY}])>0
```

## Zabbix Template

The included `zabbix_template_docker_swarm.yaml` (Zabbix 7.4+) provides:

### Discovery rules

| Rule | Interval | Items | Graphs | Triggers |
|---|---|---|---|---|
| Service Discovery | 5m | desired replicas, running replicas, failed tasks, last restart | replicas, failed tasks | replica mismatch, task failure, restart |
| Stack Discovery | 10m | health JSON, health %, unhealthy count | health % | unhealthy services, health < 100% |
| Node Discovery | 10m | status JSON, availability, state | — | not ready, availability changed |
| Replica Discovery | 5m | stats JSON, CPU %, CPU ns, mem bytes, mem %, mem limit | CPU %, memory | high CPU (>80% 5m avg), high memory (>90%) |

### Embedded dashboard (3 pages)

| Page | Contents |
|---|---|
| Overview | Active problems, node status table, stack health table, service trigger overview grid |
| Services | Replica count graphs (running vs desired), failed task graphs, stack health % graphs |
| Resources | CPU % graphs per replica, memory graphs per replica (usage vs limit), resource alerts |

The dashboard is a **template dashboard** — it is automatically scoped to each host the template is linked to, no manual widget configuration needed.

## Troubleshooting

### Common issues

| Symptom | Cause | Fix |
|---|---|---|
| Permission denied | Zabbix user cannot read Docker socket | `sudo usermod -aG docker zabbix` |
| No services found | Swarm not running or no services | `docker service ls` |
| Stack not detected | Missing `com.docker.stack.namespace` label | Deploy via `docker stack deploy` |
| Replica stats empty | Plugin not running on worker node | Install plugin on all nodes |
| Global service desired count wrong | Was counting all nodes including drained | Fixed — now counts only active+ready nodes matching placement constraints |

### Debug commands

```bash
# Test Docker API directly
curl --unix-socket /var/run/docker.sock http://localhost/v1.47/services | jq .
curl --unix-socket /var/run/docker.sock http://localhost/v1.47/nodes | jq .

# Check Zabbix Agent 2 logs
sudo tail -f /var/log/zabbix/zabbix_agent2.log

# Test all metrics
zabbix_get -s localhost -k "swarm.services.discovery"
zabbix_get -s localhost -k "swarm.stacks.discovery"
zabbix_get -s localhost -k "swarm.nodes.discovery"
zabbix_get -s localhost -k "swarm.replicas.discovery"
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests if applicable
5. Submit a pull request

## License

This project is licensed under the MIT License — see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- Zabbix team for the excellent Agent 2 plugin framework
- Docker team for the comprehensive Swarm API
- Community contributors and testers
