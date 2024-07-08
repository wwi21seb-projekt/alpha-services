#!/bin/bash

# Set current jwt tokens
python fetch_token.py
echo "JWT tokens fetched Successfully!"

# Parent directory containing multiple subdirectories with test cases
PARENT_DIR="integration-tests"

# Check if the parent directory exists
if [ ! -d "$PARENT_DIR" ]; then
  echo "Parent directory $PARENT_DIR does not exist."
  exit 1
fi

# Loop through each subdirectory in the parent directory
for dir in "$PARENT_DIR"/*/; do
  if [ -d "$dir" ]; then
    echo "Running Venom tests in directory: $dir"
    venom run "$dir" --var-from-file $PARENT_DIR/variables.yaml --output-dir $dir/logs
  fi
done

echo "Venom scripts were executed."
