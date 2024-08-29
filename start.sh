#!/bin/bash

# Define the .env file path
ENV_FILE=".env"

# Function to get the BINARY_NAME from the .env file
get_binary_name() {
  grep "^BINARY_NAME=" "$ENV_FILE" | cut -d'=' -f2
}

# Function to get the PID from the .env file
get_pid() {
  grep "^PID=" "$ENV_FILE" | cut -d'=' -f2
}

# Function to stop the process
stop_process() {
  local pid="$1"
  if [ -z "$pid" ]; then
    echo "No PID found to stop."
    return 1
  fi

  echo "Stopping process with PID $pid..."
  kill "$pid" 2>/dev/null
  if [ $? -eq 0 ]; then
    echo "Process $pid stopped."
  else
    echo "Failed to stop process $pid or process does not exist."
    return 1
  fi
}

# Function to start the process
start_process() {
  local binary_path="$1"
  echo "Starting the application..."
  "$binary_path" &
  local new_pid=$!
  echo "Application started with PID $new_pid"
  # Update the PID in the .env file
  update_env_file "$new_pid"
}

# Function to update the .env file with the new PID
update_env_file() {
  local new_pid="$1"
  # Create a temporary file
  local temp_file=$(mktemp)

  # Write lines from the original file, replacing the PID line
  while IFS= read -r line; do
    if [[ $line == PID=* ]]; then
      echo "PID=$new_pid" >> "$temp_file"
    else
      echo "$line" >> "$temp_file"
    fi
  done < "$ENV_FILE"

  # If no PID line existed, append the new one
  if ! grep -q "^PID=" "$ENV_FILE"; then
    echo "PID=$new_pid" >> "$temp_file"
  fi

  # Replace the original .env file with the temporary file
  mv "$temp_file" "$ENV_FILE"
}

# Function to build the binary
build_binary() {
  local binary_name="$1"
  echo "Building binary $binary_name..."
  go build -o "$binary_name" .
  if [ $? -ne 0 ]; then
    echo "Failed to build binary $binary_name."
    exit 1
  fi
  echo "Binary $binary_name built successfully."
}

# Main script logic
BINARY_NAME=$(get_binary_name)
if [ -z "$BINARY_NAME" ]; then
  echo "BINARY_NAME not found in $ENV_FILE."
  exit 1
fi

# Define the path to the binary using BINARY_NAME
BINARY_PATH="./$BINARY_NAME"

# Check if binary exists
if [ ! -f "$BINARY_PATH" ]; then
  echo "Binary $BINARY_NAME not found. Building..."
  build_binary "$BINARY_NAME"
fi

# Get the PID from the .env file
PID=$(get_pid)

# Stop the existing process if running
if [ -n "$PID" ]; then
  stop_process "$PID"
else
  echo "No PID found in $ENV_FILE. Starting a new process."
fi

# Start the process
start_process "$BINARY_PATH"

echo "Operation completed."
