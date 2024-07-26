#!/bin/bash

# Constants
URL="https://alpha.c930.net/api/users/login"
YAML_FILE="integration-tests/variables.yaml"

# Function to get user data from YAML file
get_userdata() {
  USERNAME1=$(yq eval '.username1' "$YAML_FILE")
  PASSWORD1=$(yq eval '.password1' "$YAML_FILE")
  USERNAME2=$(yq eval '.username2' "$YAML_FILE")
  PASSWORD2=$(yq eval '.password2' "$YAML_FILE")
}

# Function to get JWT token
get_jwt_token() {
  RESPONSE=$(curl -s -X POST -H "Content-Type: application/json" -d "{\"username\": \"$1\", \"password\": \"$2\"}" "$URL")
  if [ $(echo "$RESPONSE" | jq -r '.token') != "null" ]; then
    echo "$RESPONSE" | jq -r '.token'
  else
    echo "Failed to obtain JWT token" >&2
    exit 1
  fi
}

# Function to update YAML file with token
update_yaml_file_with_token() {
  yq eval ".jwt$1 = \"$2\"" -i "$YAML_FILE"
}

# Main script
get_userdata

TOKEN1=$(get_jwt_token "$USERNAME1" "$PASSWORD1")
update_yaml_file_with_token 1 "$TOKEN1"

TOKEN2=$(get_jwt_token "$USERNAME2" "$PASSWORD2")
update_yaml_file_with_token 2 "$TOKEN2"

echo "JWT tokens updated successfully."
