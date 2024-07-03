#!/bin/sh

# This script is used to run go mod tidy for every service.
# Usage example: ./scripts/tidy-services.sh

services=(
    "api-gateway"
    "chat-service"
    "notification-service"
    "post-service"
    "user-service"
)

# Ensure we are in correct directory
cd src/api-gateway || exit

# Update the alpha-shared library for each service
for service in "${services[@]}"; do
    echo "Tidy $service ..."
    cd ../"$service" || exit
    go mod tidy
    cd - || exit
done

echo "All services have been tidied"