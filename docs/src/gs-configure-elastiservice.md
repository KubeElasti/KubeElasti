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
---

# Configure ElastiService

To enable scale to 0 on any supported resource, we will need to create an ElastiService custom resource for that resource. 

An ElastiService custom resource has the following structure:

```yaml title="elasti-service.yaml" linenums="1"
apiVersion: elasti.truefoundry.com/v1alpha1
kind: ElastiService
metadata:
  name: <service-name> # (1)
  namespace: <service-namespace> # (2)
spec:
  minTargetReplicas: <min-target-replicas> # (3)
  service: <service-name>
  probeResponse: # (4) Optional
    - method: GET # (5)
      path: # (6)
        type: PathPrefix
        value: /healthz
      headers: # (7) Optional
        - name: X-Probe-Type
          value: lb
      queryParams: # (8) Optional
        - name: source
          value: healthcheck
      response:
        status: 200 # (9)
        body: '{"ok":true}' # (10)
  cooldownPeriod: <cooldown-period> # (11)
  scaleTargetRef:
    apiVersion: <apiVersion> # (12)
    kind: <kind> # (13)
    name: <deployment-or-rollout-or-statefulset-name> # (14)
  enabledPeriod: # (22) Optional
    schedule: <cron-schedule> # (23)
    duration: <duration> # (24)
  triggers:
    - type: <trigger-type> # (15)
      metadata:
        query: <query> # (16)
        serverAddress: <server-address> # (17)
        threshold: <threshold> # (18)
        uptimeFilter: <uptime-filter> #(19)
  autoscaler:
    name: <autoscaler-object-name> # (20)
    type: <autoscaler-type> # (21)
```

1. Replace it with the service you want managed by elasti.
2. Replace it with the namespace of the service.
3. Replace it with the min replicas to bring up when first request arrives. Minimum: 1
4. **Optional**: Configure synthetic responses that the resolver can serve directly while the target workload is scaled to zero. This is useful for load balancer health checks and similar probes.
5. Match the HTTP method. If omitted, any method matches.
6. Match the request path using `Exact`, `PathPrefix`, or `RegularExpression`. If omitted, it defaults to a path prefix of `/`.
7. **Optional**: Match request headers. All listed headers must match.
8. **Optional**: Match query parameters. All listed query params must match.
9. HTTP status code to return when this rule matches. Supported values are `200`, `204`, `400`, `401`, `403`, `404`, `500`, `502`, `503`, and `504`.
10. JSON response body returned by the resolver. The resolver serves it with `Content-Type: application/json; charset=utf-8`.
11. Replace it with the cooldown period to wait after scaling up before considering scale down. Default: 900 seconds (15 minutes) | Maximum: 604800 seconds (7 days) | Minimum: 1 second (1 second)
12. ApiVersion should be `apps/v1` if you are using `Deployment` or `StatefulSet` or `argoproj.io/v1alpha1` in case you are using argo-rollouts.
13. Kind should be either `Deployment` or `Rollout`(in case you are using Argo Rollouts) or `StatefulSet`
14. Name should exactly match the name of the deployment or rollout or statefulset.
15. Replace it with the trigger type. Currently, KubeElasti supports only one trigger type - `prometheus`.
16. Replace it with the trigger query. In this case, it is the number of requests per second.
17. Replace it with the trigger server address. In this case, it is the address of the prometheus server.
18. Replace it with the trigger threshold. In this case, it is the number of requests per second.
19. Replace it with the uptime filter of your TSDB instance. Default: `container="prometheus"`.
20. Replace it with the autoscaler name. In this case, it is the name of the KEDA ScaledObject.
21. Replace it with the autoscaler type. In this case, it is `keda`.
22. **Optional**: Define when scale-to-zero is active. Omit this field for always-on behavior.
23. Replace with a 5-item cron expression (minute hour day month weekday) in UTC.
24. Replace with a duration string (e.g., "8h", "24h").

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
    - Minimum: 1 seconds (1 second)
- `probeResponse`: **Optional** list of probe match rules that return a synthetic response directly from the resolver while the target is scaled to zero
    - Rules are evaluated in order
    - First match wins
    - Requests served by `probeResponse` do not trigger scale-up
- `triggers`: List of conditions that determine when to scale down (currently supports only Prometheus metrics)
- `autoscaler`: **Optional** integration with an external autoscaler (HPA/KEDA) if needed
    - `<autoscaler-type>`: keda
    - `<autoscaler-object-name>`: Name of the KEDA ScaledObject

---

## Configuration Explanation

The section below explains how the different configuration options are used in KubeElasti.

### **1. scaleTargetRef: Which service KubeElasti should manage**

This is defined using the `scaleTargetRef` field in the spec. 

- `scaleTargetRef.kind`: should be either be  `Deployment` or `Rollout` or `StatefulSet` (in case you are using Argo Rollouts). 
- `scaleTargetRef.apiVersion` should be `apps/v1` if you are using `Deployment` or `StatefulSet` or `argoproj.io/v1alpha1` in case you are using argo-rollouts.  
- `scaleTargetRef.name`: name of deployment/rollout/statefulset.

<br>

### **2. Triggers: When to scale down the service to 0**

This is defined using the `triggers` field in the spec. Currently, KubeElasti supports only one trigger type - `prometheus`. 
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
    query: sum(rate(nginx_ingress_controller_nginx_process_requests_total[1m])) or vector(0)
    serverAddress: http://kube-prometheus-stack-prometheus.monitoring.svc.cluster.local:9090
    threshold: 0.5
```

<br>

### **3. ProbeResponse: Answer health checks without scaling up**

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

### **4. Scalers: How to scale up the service to 1**

Once the service is scaled down to 0, we also need to pause the current autoscaler to make sure it doesn't scale up the service again. While this is not a problem with HPA, Keda will scale up the service again since the min replicas is 1. Hence, KubeElasti needs to know about the **KEDA** ScaledObject so that it can pause it. This information is provided in the `autoscaler` field of the ElastiService. Currently, the only supported autoscaler type is **keda**.

```yaml
autoscaler:
  name: <autoscaler-object-name>
  type: keda
```

<br>

### **5. CooldownPeriod: Minimum time (in seconds) to wait after scaling up before considering scale down**

As soon as the service is scaled down to 0, KubeElasti **resolver** will start accepting requests for that service. On receiving the first request, it will scale up the service to `minTargetReplicas`. Once the pod is up, the new requests are handled by the service pods and do not pass through the elasti-resolver. The requests that came before the pod scaled up are held in memory of the elasti-resolver and are processed once the pod is up.

We can configure the `cooldownPeriod` to specify the minimum time (in seconds) to wait after scaling up before considering scale down.

<br>

### **6. EnabledPeriod: Control when scale-to-zero is active (Optional)**

The `enabledPeriod` field allows you to define specific time windows when the scale-to-zero policy should be active. Outside of these periods, KubeElasti will maintain the service at `minTargetReplicas` and prevent scale-down. This is useful for scenarios like:

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
