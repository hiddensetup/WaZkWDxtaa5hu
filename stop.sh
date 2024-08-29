#!/bin/bash

# Define the path to the .env file
ENV_FILE=".env"

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

# Main script logic
PID=$(get_pid)

# Stop the existing process if running
if [ -n "$PID" ]; then
  stop_process "$PID"
else
  echo "No PID found in $ENV_FILE."
fi

echo "Operation completed."
