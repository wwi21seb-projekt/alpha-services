#!/bin/sh

# This script is used to bump the alpha-shared library for
# every service. Usage example: ./scripts/uptake-shared.sh v0.19.0

services=(
    "api-gateway"
    "chat-service"
    "notification-service"
    "post-service"
    "user-service"
)

# Validate the input (needs to be in format v<number>.<number>.<number>)
if [ -z "$1" ] || [[ ! "$1" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "Please provide the version of alpha-shared to update to"
    exit 1
fi

# Ensure we are in correct directory
cd src/api-gateway || exit

# Update the alpha-shared library for each service
for service in "${services[@]}"; do
    echo "Updating $service to $1 ..."
    cd ../"$service" || exit
    go get -u github.com/wwi21seb-projekt/alpha-shared@"$1" && go mod tidy
    cd - || exit
done

echo "All services have been updated to $1"
