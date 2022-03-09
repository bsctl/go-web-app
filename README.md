# A simple demo webapp
Clone this repo on a local machine where `kubectl` is configured against the target Kubernetes cluster. The image is built from the provided `Dockerfile`:

```
docker build -t go-web-app:latest .
Once the image is built, push it on your preferred repository.
```

## Deployment
Use this webapp to demostrate the deployment strategies in Kubernetes.

The app exposes ports:

- 8080 for http requests on the "/" path.
- 8080 for readiness and liveness probes on the "/ready", and "live" paths, respectively.
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

Then use Prometheus and Grafana to display the number of http requests received and ordered by versions.

In Grafana, add a Prometheus data source url

    http://prometheus-server

And use the following query to see the requests ordered by `{{version}}`

    sum(rate(http_requests_total{run="webapp"}[5m])) by (version)

## Horizontal Pod Autoscaler Walkthrough
Use this webapp for an introduction to the HPA - Horizontal Pod Autoscaler. Refer to [autoscaler walkthrough](./autoscaler/README.md).