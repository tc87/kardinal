[![Docker Hub](https://img.shields.io/badge/dockerhub-images-important.svg?logo=docker)](https://hub.docker.com/u/kurtosistech)

# Kardinal

![Kardi B](https://kardinal.dev/_next/static/media/kardinal-orange.65ea335b.png)

Kardinal is a revolutionary traffic control and data isolation layer that enables engineers to safely do development and QA work directly in production. Say goodbye to maintaining multiple environments and hello to faster, more efficient development workflows.

## What is Kardinal?

Kardinal injects production data and service dependencies into your dev and test workflows safely and securely. Instead of spinning up ephemeral environments with mocked services, fake traffic, and fake data, developers using Kardinal can put their service directly into the production environment to see how it works... without risking the stability of that environment.

Key features:
- Develop and test directly in production without risk
- Catch bugs that "only appear in prod" faster
- Stop maintaining multiple environments - do it all in production
- Lighter-weight dev workflow: reuse deployed services
- Implement isolated dev sandbox flows with maximum dev-prod parity
- Control data and traffic access throughout the software development lifecycle with maturity gates

## How it Works

Kardinal uses traffic flow controls and a data isolation layer to protect production while you're developing. It achieves this by rethinking the idea of isolated "environments" and replacing them with isolated traffic flows within the production environment.

To use Kardinal, just drop the Kardinal sidecars into your production environment. Then run:

```bash
# Create a dev flow
kardinal create-dev-flow <service-name> <dev-image-tag>
```

This creates a dev flow for your service with access to all the data, traffic, and services in your production environment, while ensuring complete isolation and safety.

## Developing instructions

1. Enter the dev shell and start the local cluster:

```bash
nix develop
```

2. You're also likely to use a local k8s, in this case minikube is available to use:

```bash
kubectl config set-context minikube
minikube start --driver=docker --cpus=10 --memory 8192 --disk-size 32g
minikube addons enable ingress
minikube addons enable metrics-server
istioctl install --set profile=demo -y
minikube dashboard
```

On a second terminal, start the tunnel:

```bash
minikube tunnel
```

## Deploying Kardinal Manager to local cluster

You can use tilt deploy and keeping the image hot-reloading:

```bash
tilt up
```

Or manually build it:

```bash
# First set the docker context to minikube
eval $(minikube docker-env)
docker load < $(nix build ./#kardinal-manager-container --no-link --print-out-paths)
kubectl apply -f kardinal-manager/deployment
```

## Deploying Redis Overlay Service to local cluster

Building and loading image into minikube:

```bash
# First set the docker context to minikube
eval $(minikube docker-env)
docker load < $(nix build ./#redis-proxy-overlay-container --no-link --print-out-paths)
```

To build and run the service directly:

```bash
nix run ./#redis-proxy-overlay
```

## Publishing multi-arch images

To publish multi-arch images, you can use the following command:

```bash
$(nix build .#publish-<SERVICE_NAME>-container --no-link --print-out-paths)/bin/push

# For instance, to publish the redis proxy overlay image:
$(nix build .#publish-redis-proxy-overlay-container --no-link --print-out-paths)/bin/push
```

## Running Kardinal CLI

To build and run the service directly:

```bash
nix run ./#kardinal-cli
```

### Regenerate gomod2nix.toml

You will need to do this every time a `go.mod` file is edited

```bash
nix develop
gomod2nix generate
```
