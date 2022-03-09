# Horizontal Pod Autoscaler Walkthrough

Horizontal Pod Autoscaler automatically scales the number of Pods in a replication controller, deployment, replica set or stateful set based on observed CPU utilization

## Simple HPA
Start a webapp deployment and expose it as a service.


<details><summary>webapp-deploy.yaml</summary>

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
  name: webapp
  namespace:
spec:
  replicas: 1
  selector:
    matchLabels:
      run: webapp
  strategy:
    rollingUpdate:
      maxSurge: 1
      maxUnavailable: 0
    type: RollingUpdate
  progressDeadlineSeconds: 300
  revisionHistoryLimit: 3
  template:
    metadata:
      labels:
        run: webapp
    spec:
      containers:
      - image: bsctl/go-web-app:latest
        name: webapp
        env:
        - name: VERSION
          value: v1.0.0
        resources:
          requests:
            cpu: 100m
        ports:
        - name: http
          containerPort: 8080
          protocol: TCP
        - name: metrics
          containerPort: 9090
          protocol: TCP
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
            scheme: HTTP
          initialDelaySeconds: 20
          periodSeconds: 10
        livenessProbe:
          httpGet:
            path: /live
            port: 8080
            scheme: HTTP
          periodSeconds: 15
---
apiVersion: v1
kind: Service
metadata:
  name: webapp
  labels:
    run: webapp
  namespace:
spec:
  ports:
  - name: http
    protocol: TCP
    port: 80
    targetPort: 8080
  - name: metrics
    protocol: TCP
    port: 9090
    targetPort: 9090
  selector:
    run: webapp
  type: ClusterIP
  sessionAffinity: None
```

</details>

```
kubectl apply -f webapp-deploy.yaml
```

Create a Horizontal Pod Autoscaler:

<details><summary>webapp-hpa.yaml</summary>

```yaml
apiVersion: autoscaling/v1
kind: HorizontalPodAutoscaler
metadata:
  name: webapp-hpa
  namespace: 
spec:
  maxReplicas: 10
  minReplicas: 1
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: webapp
  targetCPUUtilizationPercentage: 80
```
</details>

```
kubectl apply -f webapp-hpa.yaml
```

Now, we will see how the autoscaler reacts to increased load. We will start a container, and send an infinite loop of queries to the webapp service (please run it in a different terminal):

```
kubectl run vegeta --rm --attach --restart=Never --image="peterevans/vegeta" -- sh -c "echo 'GET http://webapp:80/load' | vegeta attack -duration 120s -connections 10  -rate 1 | vegeta report"
```

Within a minute or so, we should see the higher CPU load by executing:

```
kubectl get hpa -w
NAME     REFERENCE          TARGET      MINPODS   MAXPODS   REPLICAS   AGE
webapp   Deployment/webapp  305% / 50%  1         10        1          3m
webapp   Deployment/webapp  427% / 50%  1         10        4          4m
```

Stop the load and then verify the result state, after a minute or so:

```
kubectl get hpa
NAME     REFERENCE           TARGET       MINPODS   MAXPODS   REPLICAS   AGE
webapp   Deployment/webapp   0% / 50%     1         10        1          11m

```

Here CPU utilization dropped to 0, and so HPA autoscaled the number of replicas back down to 1.


## Algorithm details
From the most basic perspective, the `HorizontalPodAutoscaler` controller operates on the ratio between desired metric value and current metric value:

```
desiredReplicas = ceil[currentReplicas * ( currentMetricValue / desiredMetricValue )]
```

In our example above, the `targetCPUUtilizationPercentage` is set to 80%. But 80% of what, exactly?

The process running inside a container is guaranteed the amount of CPU requested through the resource requests specified for the container. But at times when no other processes need CPU, the process may use all the available CPU on the node.

When say a pod is consuming 80% of the CPU, it’s not clear if it means 80% of the node’s CPU, 80% of the pod’s guaranteed CPU (the resource request), or 80% of the hard limit configured for the pod through resource limits.

As far as the HPA is concerned, only the pod’s guaranteed CPU amount, i.e. the CPU requests is important when determining the CPU utilization of a pod. The autoscaler compares the pod’s actual CPU consumption and its CPU requests, which means the pods need to have CPU requests set for the HPA to determine the CPU utilization percentage.

## Autoscaling on multiple metrics
Introduce additional metrics to use when autoscaling the webapp deploy by making use of the `autoscaling/v2beta2` API version. With Kubernetes 1.23+, use `autoscaling/v2` API version:

<details>
<summary>webapp-hpav2.yaml</summary>

```yaml
apiVersion: autoscaling/v2beta2
kind: HorizontalPodAutoscaler
metadata:
  name: webapp-hpav2
  namespace:
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: webapp
  minReplicas: 1
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 80
```

</details>

Notice that the old `targetCPUUtilizationPercentage` field in `autoscaling/v1` has been replaced in `autoscaling/v2beta2` with an array called `metrics`. The CPU utilization metric is a resource metric, since it is represented as a percentage of a resource specified on pod containers.

You can specify other resource metrics besides CPU. By default, the only other supported resource metric is `memory`. These resources do not change names from cluster to cluster, and should always be available, as long as the `metrics.k8s.io` API is available on Kubernetes APIs Server. To achieve that, please, make sure you have installed the metric server on your cluster.

```
$ kubectl -n kube-system get deploy | grep metrics-server
metrics-server-v0.4.5           1/1     1            1           8d
```

See the metrics

```
kubectl get --raw /apis/metrics.k8s.io/v1beta1 | jq
```

<details>

```json
{
  "kind": "APIResourceList",
  "apiVersion": "v1",
  "groupVersion": "metrics.k8s.io/v1beta1",
  "resources": [
    {
      "name": "nodes",
      "singularName": "",
      "namespaced": false,
      "kind": "NodeMetrics",
      "verbs": [
        "get",
        "list"
      ]
    },
    {
      "name": "pods",
      "singularName": "",
      "namespaced": true,
      "kind": "PodMetrics",
      "verbs": [
        "get",
        "list"
      ]
    }
  ]
}
```

</details>

These APIs can be queried by

```
kubectl -n webapp top pods --v=6

I0226 19:41:25.801610   18124 round_trippers.go:454] GET https://35.205.68.114/apis/metrics.k8s.io/v1beta1/namespaces/webapp/pods 200 OK in 74 milliseconds
NAME                      CPU(cores)   MEMORY(bytes)   
webapp-56b65c4775-dm5tr   1m           4Mi             
webapp-56b65c4775-lxp94   1m           3Mi             
webapp-56b65c4775-pcx6n   1m           4Mi             
```

When multiple metrics are specified in HPA, the HPA will calculate proposed replica counts for each metric, and then choose the one with the highest replica count.

## Custom metrics
Scaling on CPU or memory metrics is not used by all systems and does not appear to be as reliable. For most web backend systems, elastic scaling based on RPS (requests per second) to handle bursts of traffic is more reliable.


### Prometheus
Prometheus is a popular open source monitoring system that provides access to real-time traffic load metrics, so we’ll be trying out a custom metric based on Prometheus for elastic scaling.

The HPA is to obtain metrics data from Prometheus. Here the Prometheus Adapter component is introduced, which implements the resource metrics, custom metrics and external metrics APIs and supports autoscaling/v2beta2 HPAs.

Once the metrics data is obtained, the number of examples of workloads is adjusted according to predefined rules.

### Web Applications exposing custom metrics
The webapp exposes a custom metric `http_requests_total` on the `https://webapp:9090/metrics` path. To see the metrics, forward the service

```
kubectl -n webapp port-forward  svc/webapp 9001:9090

curl http://127.0.0.1:9001/metrics

...
# HELP http_requests_total A counter for received requests
# TYPE http_requests_total counter
http_requests_total{code="200",method="get",version="v1.0.0"} 388
```

### Prometheus Adapter
Assume a Prometheus server is installed on your cluster and it is reachable as:

```yaml
  url: http://mon-kube-prometheus-stack-prometheus.monitoring-system.svc
  port: 9090
```

Grafana is optional.

To expose custom metrics APIs in your cluster, you need for the [Prometheus Adapter](https://github.com/kubernetes-sigs/prometheus-adapter) that serves the custom metrics APIs.

In order to monitor your application, you'll need to set up a ServiceMonitor pointing at the application. Assuming you've set up your Prometheus instance to use ServiceMonitors with the `run=webapp` label, create a ServiceMonitor to monitor the webapp metrics via the service

<details>
<summary>webapp-service-monitor.yaml</summary>

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  labels:
    monitoring: "true"
  name: webapp-metrics
  namespace:
spec:
  endpoints:
  - path: /metrics
    port: metrics
  selector:
    matchLabels:
      run: webapp
```

</details>

```
kubectl create -f webapp-service-monitor.yaml
```

Now, you should see your metrics appear in your Prometheus instance. Look them up via the dashboard, and make sure they have the `namespace` and `pod` labels. For example, use the query `sum(rate(http_requests_total[3m]))` in you prometheus dashboard.

Thanks to the Prometheus Adapter, we can turn this query into an API custom metric.

Install the Prometheus Adapter 

```
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update
helm -n monitoring-system install prom-adapter prometheus-community/prometheus-adapter --values values-prometheus-adapter.yaml
```

In addition to configuring how the Prometheus server is accessed, it is also important to configure the rules for calculating custom metrics, telling the adapter how to get the metrics from Prometheus and calculate the metrics we need:

<details>
<summary>values-prometheus-adapter.yaml</summary>

```yaml
prometheus:
  url: http://mon-kube-prometheus-stack-prometheus.monitoring-system.svc
  port: 9090
metricsRelistInterval: 60s
logLevel: 4
# for details on rules configuration, please sse:
# https://github.com/kubernetes-sigs/prometheus-adapter/blob/master/docs/config.md
rules:
  default: false
  custom:
   - seriesQuery: '{__name__=~"^http_requests_total$",container!="POD",namespace!="",pod!=""}'
     resources:
       overrides:
         namespace: { resource: "namespace" }
         pod: { resource: "pod" }
     name:
       matches: "http_requests_total"
       as: "http_requests_per_seconds"
     metricsQuery: sum(rate(<<.Series>>{<<.LabelMatchers>>}[3m])) by (<<.GroupBy>>)
```

</details>

After the promethues-adapter pod has successfully run, you can access the `custom.metrics.k8s.io` with an APIs request.

```json
kubectl get --raw /apis/custom.metrics.k8s.io/v1beta1 | jq

{
  "kind": "APIResourceList",
  "apiVersion": "v1",
  "groupVersion": "custom.metrics.k8s.io/v1beta1",
  "resources": [
    {
      "name": "namespaces/http_requests_per_seconds",
      "singularName": "",
      "namespaced": false,
      "kind": "MetricValueList",
      "verbs": [
        "get"
      ]
    },
    {
      "name": "pods/http_requests_per_seconds",
      "singularName": "",
      "namespaced": true,
      "kind": "MetricValueList",
      "verbs": [
        "get"
      ]
    }
  ]
}
```

Because of the Prometheus Adapter configuration, the cumulative metric `http_requests_total` has been converted into a rate metric, `http_requests_per_seconds`, which measures requests per second over a 3 minute interval. The value should currently be close to zero, since there's no traffic to webapp, except for the regular metrics collection from Prometheus and `kubelet` probes:

```
kubectl get --raw "/apis/custom.metrics.k8s.io/v1beta1/namespaces/webapp/pods/*/http_requests_per_seconds" | jq
```

<details>

```json
{
  "kind": "MetricValueList",
  "apiVersion": "custom.metrics.k8s.io/v1beta1",
  "metadata": {
    "selfLink": "/apis/custom.metrics.k8s.io/v1beta1/namespaces/webapp/pods/%2A/http_requests_per_seconds"
  },
  "items": [
    {
      "describedObject": {
        "kind": "Pod",
        "namespace": "webapp",
        "name": "webapp-56b65c4775-dm5tr",
        "apiVersion": "/v1"
      },
      "metricName": "http_requests_per_seconds",
      "timestamp": "2022-02-26T19:05:01Z",
      "value": "166m",
      "selector": null
    },
    {
      "describedObject": {
        "kind": "Pod",
        "namespace": "webapp",
        "name": "webapp-56b65c4775-lxp94",
        "apiVersion": "/v1"
      },
      "metricName": "http_requests_per_seconds",
      "timestamp": "2022-02-26T19:05:01Z",
      "value": "166m",
      "selector": null
    },
    {
      "describedObject": {
        "kind": "Pod",
        "namespace": "webapp",
        "name": "webapp-56b65c4775-pcx6n",
        "apiVersion": "/v1"
      },
      "metricName": "http_requests_per_seconds",
      "timestamp": "2022-02-26T19:05:01Z",
      "value": "166m",
      "selector": null
    }
  ]
}
```

</details>


> Here value: `166m`, identifies milli-requests per seconds, so `166m` here means `0.17` requests per second.

### Autoscaler

Create the configuration of the HPA is as follows:

* the minimum and maximum number of replicas is set to `1` and `10` respectively

* specify the metric `http_requests_per_seconds`, the type `Pods` and the target value `50`

* the average per pod is `50`

<details>
<summary>webapp-hpav2-custom-metrics.yaml</summary>

```yaml
apiVersion: autoscaling/v2beta2
kind: HorizontalPodAutoscaler
metadata:
  name: webapp-hpav2-custom-metrics
  namespace:
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: webapp
  minReplicas: 1
  maxReplicas: 10
  metrics:
    - type: Pods
      pods:
        metric:
          name: http_requests_per_seconds
        target:
          type: AverageValue
          averageValue: 50000m
```

</details>

Create the autoscaler:

```
kubectl apply -f webapp-hpav2-custom-metrics.yaml
```

For example, with `300` request per second, the expected number of replicas will be `300/50=6`.

### Testing

The tool used for testing `vegeta` can specify the request per second:

```
kubectl run vegeta --rm --attach --restart=Never \
--image="peterevans/vegeta" -- \
sh -c "echo 'GET http://webapp:80' | vegeta attack -duration 280s -connections 10 -rate 300 | vegeta report"
```

On another terminal window, check the progression of autoscaling

```
kubectl get hpa webapp-hpav2-custom-metrics -w
NAME                          REFERENCE           TARGETS   MINPODS   MAXPODS   REPLICAS      AGE
webapp-hpav2-custom-metrics   Deployment/webapp   166m/50      1         100       1          61m
webapp-hpav2-custom-metrics   Deployment/webapp   52500m/50    1         100       1          62m
webapp-hpav2-custom-metrics   Deployment/webapp   112506m/50   1         100       1          62m
webapp-hpav2-custom-metrics   Deployment/webapp   112506m/50   1         100       3          62m
webapp-hpav2-custom-metrics   Deployment/webapp   172506m/50   1         100       3          63m
webapp-hpav2-custom-metrics   Deployment/webapp   86267m/50    1         100       4          63m
webapp-hpav2-custom-metrics   Deployment/webapp   77649m/50    1         100       4          63m
webapp-hpav2-custom-metrics   Deployment/webapp   78026m/50    1         100       5          63m
webapp-hpav2-custom-metrics   Deployment/webapp   97550m/50    1         100       5          64m
webapp-hpav2-custom-metrics   Deployment/webapp   73384m/50    1         100       6          64m
webapp-hpav2-custom-metrics   Deployment/webapp   75027m/50    1         100       6          64m
webapp-hpav2-custom-metrics   Deployment/webapp   60227m/50    1         100       6          64m
webapp-hpav2-custom-metrics   Deployment/webapp   49960m/50    1         100       6          65m
webapp-hpav2-custom-metrics   Deployment/webapp   50155m/50    1         100       6          66m
webapp-hpav2-custom-metrics   Deployment/webapp   49738m/50    1         100       6          66m
webapp-hpav2-custom-metrics   Deployment/webapp   41443m/50    1         100       6          67m
webapp-hpav2-custom-metrics   Deployment/webapp   31532m/50    1         100       6          67m
webapp-hpav2-custom-metrics   Deployment/webapp   21452m/50    1         100       6          68m
webapp-hpav2-custom-metrics   Deployment/webapp   11446m/50    1         100       6          68m
webapp-hpav2-custom-metrics   Deployment/webapp   1446m/50     1         100       6          69m
webapp-hpav2-custom-metrics   Deployment/webapp   166m/50      1         100       6          71m
webapp-hpav2-custom-metrics   Deployment/webapp   166m/50      1         100       5          72m
webapp-hpav2-custom-metrics   Deployment/webapp   166m/50      1         100       4          72m
webapp-hpav2-custom-metrics   Deployment/webapp   166m/50      1         100       3          73m

```

### Conclusions
Horizontal scaling of applications based on custom metrics is more suitable for web based applications. Promeheus is a popular application monitoring system that can be used as a scaling metric with the support of the Adapter for custom metrics.
