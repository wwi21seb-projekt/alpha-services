# alpha-services

Welcome to the repository for Server Alpha! This project is structured as a monorepo, with multiple services located in their respective directories under src/<service-name>. Below is an overview of the services and how to get started with the project.

- `api-gateway`: The API Gateway service routes requests to the appropriate service, acts as the entry point for all requests, and handles authentication.
- `notification-service`: Handles sending notifications to users, including Web Push, Expo Push Notifications and Email Notifications.
- `post-service`: Manages feeds, posts, hashtags, and interactions (likes, comments, etc.).
- `user-service`: Manages users, including subscriptions and followers.
- `image-service`: Manages images, saving to a persistent storage and serving images.
- `chat-service`: Manages chats, including the creation of chats and offering real-time chats with websockets and streams.

## Technology Stack

- **Language**: Golang
- **Communication**: gRPC between services, HTTP over JSON (with Gin router) for end users in api-gateway.
- **Containerization**: Docker
- **Orchestration**: Kubernetes
- **Database**: PostgreSQL (shared), with each service having its own schema.
- **Database Migrations**: Managed using Atlas HCL (db directory).
- **Shared Libraries**: Located under github.com/wwi21seb-projekt/alpha-shared.

## Getting Started

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/)
- [Kubectl and Kind](https://kubernetes.io/docs/tasks/tools/)
- [Helm](https://helm.sh/docs/intro/install/)
- [Skaffold](https://skaffold.dev/docs/install/)
- [Go](https://golang.org/doc/install)
- [Atlas](https://atlasgo.io/getting-started)

Central to the deployment process is the Helm chart alpha-chart located in the helm/ directory. To be able to use the service, you need to copy the file `values-dev.template.yaml` into a new file `values-dev.yaml` and fill out the `MAILGUN_API_KEY` field.

```yaml
secretOverride:
  MAILGUN_API_KEY: "" # (Ask Luca for the key)
```

You can get the `MAILGUN_API_KEY` from Luca, the `VAPID_PUBLIC_KEY` and `VAPID_PRIVATE_KEY` can be generated on the following website for development purposes: [VAPID Key Generator](https://web-push-codelab.glitch.me/).

To override other keys:

```yaml
secretOverride:
  VAPID_PUBLIC_KEY: "YourKeyGoesHere"
  VAPID_PRIVATE_KEY: "YourKeyGoesHere"
  JWT_PRIVATE_KEY_BASE64: "WW91cktleUdvZXNIZXJl"
  JWT_PUBLIC_KEY_BASE64: "WW91cktleUdvZXNIZXJl"
```

Alternatively, replace the keys in your files folder in the helm/alpha-chart/ directory. Helper scripts in the scripts/ directory can generate and base64 encode keys.

### Setup

#### Cloning the repository

```bash
# Using GitHub CLI
gh repo clone wwi21seb-projekt/alpha-services
# Or via git
git clone <ssh or https link>
```

#### Preparing the cluster

```bash
kind create cluster
cd helm/alpha-chart
helm dependency build
```

#### Running the services

##### With Skaffold

```bash
# From the root directory
skaffold dev
```

Access:

- API Gateway: localhost:8080
- Database: localhost:5432
- Jaeger (if enabled): localhost:16686
- Grafana (if enabled): localhost:3000 (default user: admin, password: prom-operator)

Stop services with `Ctrl+C`.

#### Just with Helm

```bash
cd helm/alpha-chart
helm upgrade --install alpha-release . -f values-dev.yaml
```

Run the command again to update the services if there are updates to the chart.

### Applying database migrations

```bash
cd db
atlas migrate apply --env=local
```

### Deleting the cluster

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

## Testing

This project uses venom for integration testing. Documentation as well as instructions for installation can be found in this github repository: https://github.com/ovh/venom?tab=readme-ov-file

Test implementations can be found in the `integration-tests` folder. To execute all of the existing tests, you can run the `test-all` script located in the `scripts` directory.

To execute individual test suites, use the following command:

```bash
venom run <directory> --output-dir <directory>/logs
```

Example: Running the Imprint Test Suite
For the imprint test suite, the command would be:

```bash
venom run integration-tests/imprint --output-dir integration-tests/imprint/logs
```
Make sure to specify the logs directory as the output directory to prevent the log files from being stored elsewhere.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
