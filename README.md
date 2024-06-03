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
- [Skaffold](https://skaffold.dev/docs/install/)
- [Go](https://golang.org/doc/install)
- [Protoc](https://grpc.io/docs/protoc-installation/) (only if you work on the API in `alpha-shared`)
- [Atlas](https://atlasgo.io/getting-started)

### Setup

1. Clone the repository with the GitHub CLI or via `git clone`.
2. Start a local Kubernetes cluster with `kind create cluster`, make sure the Docker daemon is running.
3. Run `skaffold dev` in the root directory to start the services and database.
4. Change to the `db` directory and run `atlas migrate apply --env=local` to apply the newest schema migrations.
5. The services should now be running and you can access the API Gateway at `localhost:8080` and the database at `localhost:5432`.
6. To stop the services, interrupt the `skaffold dev` process with `Ctrl+C`.
7. Optionally, you can run `kind delete cluster` to delete the local Kubernetes cluster.

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
