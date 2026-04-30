---
title: "Configure ElastiService - KubeElasti Custom Resource Setup"
description: "Learn how to configure ElastiService CRD for KubeElasti. Complete guide to setting up scale-to-zero with triggers, scalers, and resources configuration."
keywords:
  - ElastiService configuration
  - KubeElasti CRD setup
  - Kubernetes custom resource
  - scale to zero configuration
  - ElastiService yaml
  - KubeElasti triggers
icon: lucide/sliders-horizontal
---

# Configuration

## ElastiService

To enable scale to 0 on any supported resource, we will need to create an ElastiService custom resource for that resource. 

An ElastiService custom resource has the following structure:

```yaml title="elasti-service.yaml" linenums="1"
apiVersion: elasti.truefoundry.com/v1alpha1
kind: ElastiService
metadata:
  name: <service-name> # (1)
  namespace: <service-namespace> # (2)
spec:
  service: <service-name> # (3)
  minTargetReplicas: <min-target-replicas> # (4)
  cooldownPeriod: <cooldown-period> # (5)
  scaleTargetRef:
    apiVersion: <apiVersion> # (6)
    kind: <kind> # (7)
    name: <deployment-or-rollout-or-statefulset-name> # (8)
  triggers:
    - type: <trigger-type> # (9)
      metadata:
        query: <query> # (10)
        serverAddress: <server-address> # (11)
        threshold: <threshold> # (12)
        uptimeFilter: <uptime-filter> # (13)
  autoscaler: # (14) Optional
    name: <autoscaler-object-name> # (15)
    type: <autoscaler-type> # (16)
  probeResponse: # (17) Optional
    - method: GET # (18)
      path: # (19)
        type: PathPrefix
        value: /healthz
      headers: # (20) Optional
        - name: X-Probe-Type
          value: lb
      queryParams: # (21) Optional
        - name: source
          value: healthcheck
      response:
        status: 200 # (22)
        body: '{"ok":true}' # (23)
  enabledPeriod: # (24) Optional
    schedule: <cron-schedule> # (25)
    duration: <duration> # (26)
```

1. Name of the ElastiService object. Conventionally matches the name of the Kubernetes `Service` you want to manage.
2. Namespace of the Kubernetes `Service` to manage. The ElastiService must live in the same namespace.
3. Name of the existing Kubernetes `Service` whose traffic should be intercepted while the workload is at zero replicas.
4. Minimum number of replicas to bring up when the first request arrives. **Minimum: 1**.
5. Cooldown period (in seconds) to wait after scaling up before considering scale down. **Default: 900 (15 minutes) | Maximum: 604800 (7 days) | Minimum: 0 (disable cooldown).**
6. `apiVersion` of the scale target. Use `apps/v1` for `Deployment` and `StatefulSet`, or `argoproj.io/v1alpha1` for Argo Rollouts.
7. `kind` of the scale target. One of `Deployment`, `StatefulSet`, or `Rollout`.
8. Name of the deployment, rollout, or statefulset to scale.
9. Trigger type. Currently only `prometheus` is supported.
10. Prometheus query that decides scale-down. KubeElasti polls this metric and compares against `threshold`.
11. Address of the Prometheus server (e.g. `http://kube-prometheus-stack-prometheus.monitoring.svc.cluster.local:9090`).
12. Numeric threshold. If the query value is **below** this for the cooldown window, the workload is scaled to zero.
13. **Optional** PromQL filter used to detect Prometheus uptime, defaults to `container="prometheus"`.
14. **Optional** integration with an external autoscaler. Currently only `keda` is wired in. KubeElasti pauses the ScaledObject while the workload is at zero so KEDA does not bring it back up.
15. Name of the KEDA `ScaledObject` to pause/resume.
16. Autoscaler type. Currently only `keda` is supported.
17. **Optional** list of probe-response rules. Matching requests are answered directly by the resolver while the workload is scaled to zero, **without** triggering scale-up.
18. **Optional** HTTP method to match. If omitted, any method matches.
19. **Optional** request path matcher. `type` is one of `Exact`, `PathPrefix`, or `RegularExpression`. If `path` is omitted, the rule defaults to a `PathPrefix` of `/`.
20. **Optional** header matchers (ANDed). All listed headers must match for the rule to fire.
21. **Optional** query-parameter matchers (ANDed). All listed params must match for the rule to fire.
22. HTTP status code to return when this rule matches. Allowed values: `200`, `204`, `400`, `401`, `403`, `404`, `500`, `502`, `503`, `504`.
23. Response body returned by the resolver. Sent with `Content-Type: application/json; charset=utf-8`.
24. **Optional** schedule that gates when scale-to-zero is active. If omitted, scale-to-zero is always active.
25. 5-field cron expression (minute hour day month weekday) in **UTC** that opens the scale-to-zero window.
26. Go duration string (for example `"8h"`, `"24h"`) that controls how long each window lasts.

The key fields to be specified in the spec are:

- `<service-name>`: Replace it with the service you want managed by elasti.
- `<service-namespace>`: Replace by namespace of the service.
- `<min-target-replicas>`: Min replicas to bring up when first request arrives.
    - Minimum: 1
- `<scaleTargetRef>`: Reference to the scale target similar to the one used in HorizontalPodAutoscaler.
- `<kind>`: Replace by `Rollout` or `Deployment` or `StatefulSet`
- `<apiVersion>`: Replace with `argoproj.io/v1alpha1` or `apps/v1`
- `<deployment-or-rollout-or-statefulset-name>`: Replace with name of the rollout or the deployment or statefulset for the service. This will be scaled up to min-target-replicas when first request comes
- `cooldownPeriod`: Minimum time (in seconds) to wait after scaling up before considering scale down.
    - Default: 900 seconds (15 minutes)
    - Maximum: 604800 seconds (7 days)
    - Minimum: 0 seconds (disable cooldown)
- `probeResponse`: **Optional** list of probe match rules that return a synthetic response directly from the resolver while the target is scaled to zero
    - Rules are evaluated in order
    - First match wins
    - Requests served by `probeResponse` do not trigger scale-up
- `triggers`: List of conditions that determine when to scale down (currently supports only Prometheus metrics)
- `autoscaler`: **Optional** integration with an external autoscaler. Currently only **KEDA** is supported. KubeElasti pauses the named `ScaledObject` while the workload is at zero so KEDA does not bring it back up. HPA does not need to be registered here — it works alongside KubeElasti without coordination.
    - `<autoscaler-type>`: `keda`
    - `<autoscaler-object-name>`: Name of the KEDA `ScaledObject`



## Specs

The section below explains how the different configuration options are used in KubeElasti.

### **1. scaleTargetRef**

Target service is defined using the `scaleTargetRef` field in the spec. 

- `scaleTargetRef.kind`: should be either be  `Deployment` or `Rollout` or `StatefulSet` (in case you are using Argo Rollouts). 
- `scaleTargetRef.apiVersion` should be `apps/v1` if you are using `Deployment` or `StatefulSet` or `argoproj.io/v1alpha1` in case you are using argo-rollouts.  
- `scaleTargetRef.name`: name of deployment/rollout/statefulset.

<br>

### **2. triggers**

Triggers to scale down the service to 0 is defined using the `triggers` field in the spec. Currently, KubeElasti supports only one trigger type - `prometheus`. 
The `metadata` section holds trigger-specific data:  

- **query** - the Prometheus query to evaluate  
- **serverAddress** - address of the Prometheus server  
- **threshold** - numeric threshold that triggers scale-down    

For example, you can query the number of requests per second and set the threshold to `0`.  
KubeElasti polls this metric every 30 seconds, and if the **value** is below the threshold it scales the service to 0.

An example trigger is as follows:

```yaml
triggers:
  - type: prometheus
    metadata:
      query: sum(rate(nginx_ingress_controller_requests[1m])) or vector(0)
      serverAddress: http://kube-prometheus-stack-prometheus.monitoring.svc.cluster.local:9090
      threshold: "0.5"
```

<br>

### **3. probeResponse**

The optional `probeResponse` field lets the resolver return a local response for matching requests while the target service remains scaled to zero. This is mainly useful for liveness checks, readiness checks, or load balancer health checks that should succeed even when no application pods are running.

Each rule can match on:

- `method`
- `path`
- `headers`
- `queryParams`

All conditions inside a rule are ANDed together, and rules are evaluated top to bottom. The first matching rule is returned directly by the resolver. If no rule matches, normal KubeElasti behavior applies and the request can trigger scale-up.

```yaml
probeResponse:
  - method: GET
    path:
      type: PathPrefix
      value: /healthz
    response:
      status: 200
      body: '{"ok":true}'
  - method: HEAD
    path:
      type: Exact
      value: /ready
    response:
      status: 204
      body: '{}'
```

Use `PathPrefix` for simple health endpoints and `RegularExpression` only when you need more advanced matching.

<br>

### **4. scalers**

Once the service is scaled down to 0, we also need to pause the current autoscaler to make sure it doesn't scale up the service again. While this is not a problem with HPA, Keda will scale up the service again since the min replicas is 1. Hence, KubeElasti needs to know about the **KEDA** ScaledObject so that it can pause it. This information is provided in the `autoscaler` field of the ElastiService. Currently, the only supported autoscaler type is **keda**.

```yaml
autoscaler:
  name: <autoscaler-object-name>
  type: keda
```

<br>

### **5. cooldownPeriod**

Minimum time (in seconds) to wait after scaling up before considering scale down. As soon as the service is scaled down to 0, KubeElasti **resolver** will start accepting requests for that service. On receiving the first request, it will scale up the service to `minTargetReplicas`. Once the pod is up, the new requests are handled by the service pods and do not pass through the elasti-resolver. The requests that came before the pod scaled up are held in memory of the elasti-resolver and are processed once the pod is up.

We can configure the `cooldownPeriod` to specify the minimum time (in seconds) to wait after scaling up before considering scale down.

<br>

### **6. enabledPeriod**

Control when scale-to-zero is active (Optional). The `enabledPeriod` field allows you to define specific time windows when the scale-to-zero policy should be active. Outside of these periods, KubeElasti will maintain the service at `minTargetReplicas` and prevent scale-down. This is useful for scenarios like:

- Only allowing scale-to-zero during night hours
- Preventing scale-down during business hours when you want services always ready
- Scheduling scale-to-zero for the weekends

**Configuration:**

```yaml
enabledPeriod:
  schedule: "0 22 * * *"   # Cron expression (5 fields)
  duration: "12h"          # How long the period lasts
```

**Fields:**

- **schedule**: A 5-item cron expression defining when the enabled period starts
  - Format: `minute hour day month weekday`
  - Uses UTC timezone
  - Examples:
    - `"0 9 * * 1-5"` - 9 AM Monday through Friday
    - `"0 0 * * *"` - Daily at midnight
    - `"*/15 8-17 * * 1-5"` - Every 15 minutes, 8 AM to 5 PM, weekdays
  - Default: `"0 0 * * *"` (daily at midnight)

- **duration**: How long the enabled period lasts from each scheduled trigger
  - Format: Go duration string (e.g., "1h", "30m", "8h", "24h")
  - Default: `"24h"`

**Behavior:**

- When `enabledPeriod` is **omitted**: Scale-to-zero is always active (default behavior)
- When `enabledPeriod` is **specified**:
  - During the enabled window: Normal scale-to-zero behavior applies
  - Outside the enabled window: Service maintains `minTargetReplicas`, scale-down is prevented

**Important Notes:**

- All times use **UTC timezone**
- The cron expression uses 5 fields (not 6 - no seconds field)
- Invalid cron expressions will log warnings and default to enabled (fail-open)
- For durations longer than 24h with daily triggers, services may always be enabled
