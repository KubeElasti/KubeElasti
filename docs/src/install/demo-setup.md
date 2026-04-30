---
title: "KubeElasti demo setup - httpbin, ElastiService, and scale-to-zero"
description: "Hands-on demo: deploy sample httpbin with ingress, define and apply an ElastiService, then verify scale-up and idle scale-down with curl."
keywords:
  - KubeElasti demo
  - ElastiService example
  - httpbin scale to zero
  - KubeElasti tutorial
  - sample deployment ingress
icon: lucide/flask-conical
---

# Demo setup

Follow these steps after [Installation](./installation.md) and when your cluster meets the [Prerequisites](./pre-requisites.md). You will run a minimal **httpbin** workload behind ingress, attach an **ElastiService**, apply it, and **curl** through the ingress to see scale-from-zero and idle scale-down.

### **1. Deploy a Target Application**

Before creating an ElastiService, you need a target deployment, service, and ingress for KubeElasti to manage. Below is a sample httpbin application you can use.

Create a file named `target-deployment.yaml`:

```yaml title="target-deployment.yaml" linenums="1"
apiVersion: v1
kind: Namespace
metadata:
  name: target
---
apiVersion: v1
kind: Service
metadata:
  name: target-deployment
  namespace: target
spec:
  type: ClusterIP
  selector:
    app: target-deployment
  ports:
    - protocol: TCP
      port: 80
      targetPort: 80
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: target-deployment
  namespace: target
spec:
  replicas: 1
  selector:
    matchLabels:
      app: target-deployment
  template:
    metadata:
      labels:
        app: target-deployment
    spec:
      containers:
        - name: target-deployment
          image: kennethreitz/httpbin
          ports:
            - containerPort: 80
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: httpbin-ingress
  namespace: target
  annotations:
    nginx.ingress.kubernetes.io/rewrite-target: /$2
    nginx.ingress.kubernetes.io/service-upstream: "true"
    nginx.ingress.kubernetes.io/upstream-vhost: "target-deployment.target.svc.cluster.local"
spec:
  ingressClassName: nginx
  rules:
    - http:
        paths:
          - path: /httpbin(/|$)(.*)
            pathType: ImplementationSpecific
            backend:
              service:
                name: target-deployment
                port:
                  number: 80
```

The `nginx.ingress.kubernetes.io/upstream-vhost` annotation above is
important for elasti: it sets the `Host` header on the request NGINX
forwards upstream, and the resolver reads that header to decide which
target service to route to. See
[Resolver Architecture > Request Routing](../documentation/architecture/resolver.md#request-routing)
for details on how routing works and how to override the header.

Apply it:

```bash
kubectl apply -f target-deployment.yaml
```

Verify the target is running:

```bash
kubectl get pods -n target
```

<br>

### **2. Define an ElastiService**

You turn scale-to-zero on for a workload by creating an **ElastiService** object: it points at your Kubernetes `Service` and scale target, defines Prometheus **triggers**, and (optionally) links a **KEDA** `ScaledObject` so KubeElasti can pause it while the workload is at zero. For a full walkthrough of every field and option, read the [**Configuration** guide](./configure-elastiservice.md).

The manifest below matches the **httpbin** example from the previous step. Save it as `elasti-service.yaml`; you will apply it in the next section.

```yaml title="elasti-service.yaml" linenums="1"
apiVersion: elasti.truefoundry.com/v1alpha1
kind: ElastiService
metadata:
  name: target-elastiservice
  namespace: target
spec:
  service: target-deployment
  minTargetReplicas: 1
  cooldownPeriod: 5
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: target-deployment
  triggers:
    - type: prometheus
      metadata:
        query: sum(rate(nginx_ingress_controller_requests{namespace="target", ingress="httpbin-ingress"}[1m])) or vector(0)
        serverAddress: http://kube-prometheus-stack-prometheus.monitoring.svc.cluster.local:9090
        threshold: "0.01"
  probeResponse:
    - method: GET
      path:
        type: PathPrefix
        value: /healthz
      response:
        status: 200
        body: '{"ok":true}'
```

If your ingress, load balancer, or platform sends **health checks** while the workload is at **zero replicas**, add **`probeResponse`** rules for those paths. Requests that match are served by the resolver and **do not** scale the deployment up.

<br>

### **3. Apply the KubeElasti service configuration**

Apply the configuration to your Kubernetes cluster:

```bash
kubectl apply -f elasti-service.yaml -n <service-namespace>
```

The pod will be scaled down to 0 replicas if there is no traffic.

<br>

### **4. Test the setup**

You can test the setup by sending requests to the nginx load balancer service.

```bash
# For NGINX
kubectl port-forward svc/nginx-ingress-ingress-nginx-controller -n nginx 8080:80

# For Istio
kubectl port-forward svc/istio-ingressgateway -n istio-system 8080:80
```

Start a watch on the target deployment.

```bash
kubectl get pods -n <NAMESPACE> -w
```

Send a request to the service.

```bash
curl -v http://localhost:8080/httpbin
```

You should see the pods being created and scaled up to 1 replica. A response from the   target service should be visible for the curl command.
The target service should be scaled down to 0 replicas if there is no traffic for `cooldownPeriod` seconds.

<br>