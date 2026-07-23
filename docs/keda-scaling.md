# KEDA Integration: Scaling Minecraft Servers with rconman

rconman exposes a Prometheus-format `/metrics` endpoint with a gauge per
configured Minecraft server indicating its **desired state** (1=running,
0=stopped). KEDA can scale each game-server workload between 0 and 1 replicas
based on this metric, letting you control server on/off state from the rconman
web UI instead of GitOps.

## Metric

```
# HELP rconman_server_desired_state Desired state of a configured Minecraft server (1=running, 0=stopped)
# TYPE rconman_server_desired_state gauge
rconman_server_desired_state{server_id="survival"} 1
rconman_server_desired_state{server_id="creative"} 0
```

The metric is unauthenticated and available at `http://<rconman-service>:8080/metrics`.

## Prerequisites

1. rconman deployed via Helm with `metrics.enabled: true` (default)
2. KEDA installed in the cluster
3. A Prometheus instance scraping rconman (or KEDA's own Prometheus scaler)

## KEDA ScaledObject Example

For each Minecraft server, create a `ScaledObject` that targets the
game-server Deployment/StatefulSet and scales 0↔1 based on the
`rconman_server_desired_state` metric filtered by `server_id`:

```yaml
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: minecraft-survival
  namespace: minecraft
spec:
  scaleTargetRef:
    name: minecraft-survival
  minReplicaCount: 0
  maxReplicaCount: 1
  pollingInterval: 30
  cooldownPeriod: 60
  triggers:
    - type: prometheus
      metadata:
        serverAddress: http://rconman-rconman.default.svc.cluster.local:8080
        metricName: rconman_server_desired_state
        threshold: "0.5"
        query: rconman_server_desired_state{server_id="survival"}
```

How it works:
- When `rconman_server_desired_state{server_id="survival"}` is 1, the metric
  exceeds the threshold (0.5) → KEDA scales the Minecraft server to 1 replica.
- When the user clicks "Stop Server" in rconman, the metric drops to 0 → KEDA
  scales the Minecraft server to 0 replicas.
- `minReplicaCount: 0` allows scale-to-zero; `maxReplicaCount: 1` ensures only
  one instance runs.

## Multiple Servers

Create one `ScaledObject` per Minecraft server, each with a different
`server_id` label in the query.
