# alpha-services

This repository contains the source code for Server Alpha. It's structured as a monorepo with multiple services, each in its own directory under `src/<service-name>`. The services are:

- `api-gateway`: The API Gateway service that routes requests to the appropriate service. Acts as the entry point for all requests and handles authentication.
- `mail-service`: The Mail Service which is responsible for sending emails to our users.
- `notification-service`: The Notification Service which is responsible for sending notifications to our users, includes Web Push and Expo Push Notifications.
- `post-service`: The Post Service which is responsible for managing feeds, posts, hashtags and interactions (likes, comments, etc.).
- `user-service`: The User Service which is responsible for managing users, including subscriptions and followers.

The services are written in Golang and gRPC is used for communication between services. The `api-gateway` service is written in Golang and uses HTTP over JSON (with Gin router) for communication with end users. All services are containerized using Docker and orchestrated using Kubernetes.

We use a shared database (PostgreSQL) for all services, but each service has its own schema if it needs a data layer. The migrations for the database schemas are managed using Atlas HCL and are listed under `db`.

Every service uses shared libraries for common functionality, such as protos, database access and utilities. These shared libraries are listed under `github.com/wwi21seb-projekt/alpha-shared`.

## Getting Started

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/)
- [Kubectl and Kind](https://kubernetes.io/docs/tasks/tools/)
- [Helm](https://helm.sh/docs/intro/install/)
- [Skaffold](https://skaffold.dev/docs/install/)
- [Go](https://golang.org/doc/install)
- [Atlas](https://atlasgo.io/getting-started)

You need to have a `.env.local` file in the `k8s/overlays/local` directory with the following content:

```env
MAILGUN_API_KEY=getItFromLuca
POSTGRES_PASSWORD=mypassword
POSTGRES_USER=myuser
POSTGRES_NAME=mydatabase
VAPID_PUBLIC_KEY=yourVapidPublicKey
VAPID_PRIVATE_KEY=yourVapidPrivateKey
```

You can get the `MAILGUN_API_KEY` from Luca, the `VAPID_PUBLIC_KEY` and `VAPID_PRIVATE_KEY` can be generated on the following website for development purposes: [VAPID Key Generator](https://web-push-codelab.glitch.me/).

### Setup

#### Cloning the repository

Clone the repository with the GitHub CLI or via `git clone`.

```bash
# Either through GitHub CLI
gh repo clone wwi21seb-projekt/alpha-services
# Or via git
git clone <ssh or https link>
```

#### Preparing the cluster

1. Create a local Kubernetes cluster with `kind create cluster --name <some_name> --config k8s/overlays/local/kind-config.yaml`.
2. Install cert-manager with `kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.6.3/cert-manager.yaml`
3. Setup ingress-nginx with `kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/release-1.10/deploy/static/provider/kind/deploy.yaml`
4. Create the `observability` namespace with `kubectl create namespace observability`, since the jaeger operator requires it.
5. Install the jaeger operator with `kubectl create -f https://github.com/jaegertracing/jaeger-operator/releases/download/v1.57.0/jaeger-operator.yaml -n observability`
6. Install the kube-prometheus-stack with:

```sh
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm install alpha-kube-prometheus-stack prometheus-community/kube-prometheus-stack --version 60.1.0
```

7. Install the otel operator with `kubectl apply -f https://github.com/open-telemetry/opentelemetry-operator/releases/download/v0.102.0/opentelemetry-operator.yaml`

> Note: It can take up to a few minutes for each step to complete. Ensure that `cert-manager` and `ingress-nginx` are running before proceeding with the rest of the steps.

#### Running the services

1. Run `skaffold dev` in the root directory to start the services and database.
2. Change to the `db` directory and run `atlas migrate apply --env=local` to apply the newest schema migrations.
3. The services should now be running and you can access the API Gateway at `localhost:8080` and the database at `localhost:5432`.
4. To stop the services, interrupt the `skaffold dev` process with `Ctrl+C`.
5. Optionally, you can run `kind delete cluster` to delete the local Kubernetes cluster. Note that this will delete all data in the cluster and you will need to prepare the cluster again [as described above](#preparing-the-cluster).

## Contributing

This project uses a feature branch workflow. To contribute, follow these steps:

1. Create a new branch from `main` with a descriptive name.
2. Make your changes and commit them using conventional commits.
3. Push your branch to the repository.
4. Open a pull request against `main` and fill out the template.
5. Wait for a review and address any comments.
6. Once approved, merge your pull request.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
