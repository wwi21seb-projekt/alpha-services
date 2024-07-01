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

Central part of the deployment process is the helm chart `alpha-chart` which is located in the `helm/` directory.
In this chart you need to fill out the file `values-dev.yaml` with your own values. 
This file is used for your own development environment.

```yaml
secretOverride:
  MAILGUN_API_KEY: "" # (Ask Luca for the key)
```

You can get the `MAILGUN_API_KEY` from Luca, the `VAPID_PUBLIC_KEY` and `VAPID_PRIVATE_KEY` can be generated on the following website for development purposes: [VAPID Key Generator](https://web-push-codelab.glitch.me/).

In case you want to overwrite fields like `VAPID_PUBLIC_KEY` or `JWT_PRIVATE_KEY_BASE64` with your own values, you can just add the following to your `values-dev.yaml`:

```yaml
secretOverride:
  VAPID_PUBLIC_KEY: "YourKeyGoesHere"
  VAPID_PRIVATE_KEY: "YourKeyGoesHere"
  JWT_PRIVATE_KEY_BASE64: "WW91cktleUdvZXNIZXJl"
  JWT_PUBLIC_KEY_BASE64: "WW91cktleUdvZXNIZXJl"
```

Alternatively, you can just replace the keys in your `files` folder in the `helm/alpha-chart/` directory.
Helper scripts are provided in the `scripts/` directory to generate the keys and base64 encode them.

Please be reminded that this is optional and only necessary if you want to overwrite the default values.

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

Create a local Kubernetes cluster with 
```bash
kind create cluster
```

#### Running the services

##### With Skaffold
```bash
skaffold dev
```
The services should now be running and you can access the API Gateway at `localhost:8080` and the database at `localhost:5432`. 
If enabled, jaeger can be accessed at `localhost:16686` and grafana at `localhost:3000`.
Please make sure the corresponding ports are not already used and are present in the `skaffold.yaml` file.
The default user for grafana is `admin` and the password is `prom-operator`.

To stop the services, interrupt the `skaffold dev` process with `Ctrl+C`.

#### Just with Helm
```bash
cd helm/alpha-chart
helm upgrade --install alpha-release . -f values-dev.yaml
```
In case there are updates to the chart, run the same command again to update the services.

### Applying database migrations

In case you need to apply database migrations, you can do so with the following command:
```bash
cd db
atlas migrate apply --env=local
```

### Deleting the cluster

After you are done with the services, you can delete the cluster with
```bash
kind delete cluster
```

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
