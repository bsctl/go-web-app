# An example of demo webapp.
Use this webapp to demostrate the deployment strategies in Kubernetes.

The app exposes three ports:

- 8080 for http requests on the "/" path.
- 8090 for readiness and liveness probes on the "/ready", and "live" paths, respectively.
- 9090 for Prometheus metrics on "/metrics" path.

The exposed metrics are the http_requests_total counter.

Deploy multiple instances in Kubernetes through a deployment

    kubectl apply -f webapp-deploy-rolling.yaml

and expose the webapp through an ingress

    kubectl apply -f webapp-svc.yaml
    kubectl apply -f webapp-ingress.yaml

Update the webapp by setting a new version string

    kubectl set env deploy webapp VERSION=v2.0.0

During the update access the webapp multiple times and see different answers coming from different versions of the application.

The use Prometheus and Grafana to disply the number of http requests received and ordered by versions.

The following query should be used to see the requests ordered by version

    sum(rate(http_requests_total{run="webapp"}[5m])) by (version)

That's all.