# 2. Architecture design

Date: 2024-05-17

## Status

In discussion

## Context

We need to define the architecture of the system to ensure that it is scalable, maintainable, and secure.

## Decision

We will use a microservices architecture to build the system. This will allow us to scale the system horizontally, and to deploy and maintain the system in a more modular way.
For simplicity, we will use a single monolithic repository to store all the services, which sounds like a contradiction, but it will allow us to manage the services in a more cohesive way.

We agreed to partition the system into the following services:

- 'api-gateway': This service will be the entry point to the system. It will be responsible for routing requests to the appropriate services, as well as for handling authentication and authorization.
- 'user-service': This service will be responsible for managing users. It will handle user registration, login, profile and subscription management.
- 'post-service': This service will be responsible for managing posts. It will handle post creation, editing, deletion, and retrieval, as well as everything related to posts, such as comments, likes, and shares.
- 'notification-service': This service will be responsible for managing notifications. It will handle storing notifications and sending them to onboarded users with web push or expo push notifications.

For that we'll use the following technologies:

- 'Golang': We'll use Go for the backend services. Go is a statically typed, compiled language that is designed for building fast, reliable, and efficient software at scale.
- 'go-micro': We'll use go-micro to build the services. go-micro is a pluggable RPC framework that provides the building blocks for building microservices in Go.
- 'PostgreSQL': We'll use PostgreSQL as the database for the system. PostgreSQL is a powerful, open-source relational database that is designed for scalability, reliability, and performance.
- 'Docker': We'll use Docker to containerize the services. Docker is a platform that allows us to package, distribute, and run applications in containers.
- 'Kubernetes': We'll use Kubernetes to orchestrate the containers. Kubernetes is an open-source platform that automates the deployment, scaling, and management of containerized applications.
- 'Jaeger': We'll use Jaeger to trace the requests between the services. Jaeger is an open-source, end-to-end distributed tracing system that is designed for monitoring and troubleshooting microservices-based architectures.
- 'Prometheus': We'll use Prometheus to monitor the services. Prometheus is an open-source monitoring and alerting toolkit that is designed for collecting, storing, and querying metrics from microservices-based architectures.

As for the communication between the services, we'll use gRPC. gRPC is a high-performance, open-source, universal RPC framework that is designed for building efficient, scalable, and reliable microservices.
The API Gateway will expose a RESTful (or more like JSON over HTTP) API to the clients, and will translate the requests to gRPC calls to the services.

The API Gateway itself will be built in a "dumb" way, meaning that it will contain no or very little business logic. It will be responsible for routing requests to the appropriate services, as well as for handling
authentication and authorization. The services themselves will contain the business logic and the Gateway will be responsible for transforming the requests and responses between the RESTful API and the gRPC services.

## Consequences

What becomes easier or more difficult to do and any risks introduced by the change that will need to be mitigated.
