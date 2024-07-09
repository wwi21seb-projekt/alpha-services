# 2. Architecture design

Date: 2024-05-17

## Status

Accepted

## Context

To ensure our system is scalable, maintainable, and secure, we need to define its architecture comprehensively. Our architecture decision will influence how we handle growth, manage services, and ensure the security of our operations.

## Decision

We will adopt a microservices architecture for our system. This approach allows us to scale horizontally and manage the system modularly. Despite using a microservices architecture, we will maintain a single monolithic repository for all services, enabling cohesive management of the services.

### Service Partitioning

- `API Gateway`: Acts as the entry point to the system. Responsible for routing requests to appropriate services, handling authentication. Authorization will be done by the services themselves.
- `User Service`: Manages users, including registration, login, profile management, and subscription management.
- `Post Service`: Manages posts, including creation, editing, deletion, retrieval, comments, likes, and shares.
- `Notification Service`: Manages notifications, stores notifications, and sends them via web push or expo push notifications. Also handles mail communication using Mailgun.
- `Image Service`: Manages images, including upload, retrieval, and deletion.

### Technologies and Tools

- `Golang`: Used for backend services. We will use the standard library along with `gin-gonic` for enhanced routing capabilities and `zap` for logging.
- `PostgreSQL`: Used as the database for the system. Each service has its own schema to ensure data isolation and manageability.
- `Docker`: Used to containerize the services, ensuring consistency across development and production environments.
- `Kubernetes`: Used to orchestrate containers, automating deployment, scaling, and management.
- `Jaeger`: Used for tracing requests between services, aiding in monitoring and troubleshooting.
- `Prometheus`: Used for monitoring services, collecting, storing, and querying metrics.
- `gRPC`: Used for communication between services, providing efficient, scalable, and reliable inter-service communication.

### API Gateway

The API Gateway will expose a RESTful API (JSON over HTTP) to clients, translating requests to gRPC calls for the appropriate services. It will:

- Contain minimal business logic.
- Handle JWT authentication and provide user information to services.
- Transform requests and responses between the RESTful API and gRPC services.
- Map proto models to HTTP over JSON schema and handle RPC status codes by returning appropriate errors from `goerrors`.

### Repositories

- `alpha-services`: Contains all services, Kubernetes resources, Helm resources, database schema (Atlas), and migrations.
- `alpha-shared`: Contains shared modules such as gRPC setup, telemetry setup, database setup, logging setup, etc.

### Database Management

Each service with a data layer will have its own PostgreSQL schema.
We use connection pooling to manage concurrent access, releasing connections as soon as they are no longer needed to prevent resource starvation.
Atlas is used for database schema management, providing automatic migration creation, execution, and changelog maintenance.

### Deployment

Each service is containerized using Docker and published using Helm.
A Helm chart creates a deployment for each service and manages external dependencies such as Jaeger for tracing and kube-prom-stack for monitoring.

### Mail Communication

The Notification Service uses Mailgun for mail distribution, ensuring reliable and scalable email communication.

## Consequences

Using a microservices architecture will allow us to scale the system horizontally, and to deploy and maintain the system in a more modular way. However, it will also introduce additional complexity, such as the need to manage inter-service communication, data consistency, and service discovery. We will need to carefully design the services to ensure that they are loosely coupled.
