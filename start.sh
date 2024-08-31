#!/bin/bash

# Define the .env file path
ENV_FILE=".env"

# Define colors
COLOR_RESET="\033[0m"
COLOR_GREEN="\033[32m"
COLOR_RED="\033[31m"
COLOR_YELLOW="\033[33m"
COLOR_CYAN="\033[36m"

# Function to create a new .env file with default values
create_env_file() {
  echo -e "${COLOR_GREEN}Creating new .env file...${COLOR_RESET}"

  # Default values
  API_KEY="EAApH1KmWEt0BO5M"
  
  # Prompt user for the middle part of the URL path
  read -p "Enter the middle path for PROXY_URL (default: 'your_path'): " MIDDLE_PATH
  MIDDLE_PATH=${MIDDLE_PATH:-"your_path"}
  PROXY_URL="https://localhost/apps/$MIDDLE_PATH/api.php"
  
  LOG_LEVEL="ERROR"
  
  read -p "Enter PORT: " PORT
  while ! [[ "$PORT" =~ ^[0-9]+$ ]]; do
    echo -e "${COLOR_RED}Invalid port. Please enter a numeric value.${COLOR_RESET}"
    read -p "Enter PORT: " PORT
  done

  AUTO_LOGIN="1"
  
  read -p "Enter BINARY_NAME: " BINARY_NAME
  while [[ -z "$BINARY_NAME" ]]; do
    echo -e "${COLOR_RED}BINARY_NAME cannot be empty. Please enter a valid binary name.${COLOR_RESET}"
    read -p "Enter BINARY_NAME: " BINARY_NAME
  done

  # Writing values to the .env file
  cat <<EOL > "$ENV_FILE"
API_KEY=$API_KEY
PROXY_URL=$PROXY_URL
LOG_LEVEL=$LOG_LEVEL
PORT=$PORT
AUTO_LOGIN=$AUTO_LOGIN
BINARY_NAME=$BINARY_NAME
EOL

  echo -e "${COLOR_GREEN}.env file created successfully.${COLOR_RESET}"
}

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
    echo -e "${COLOR_RED}No PID found to stop.${COLOR_RESET}"
    return 1
  fi

  echo -e "${COLOR_YELLOW}Stopping process with PID $pid...${COLOR_RESET}"
  kill "$pid" 2>/dev/null
  if [ $? -eq 0 ]; then
    echo -e "${COLOR_GREEN}Process $pid stopped.${COLOR_RESET}"
  else
    echo -e "${COLOR_RED}Failed to stop process $pid or process does not exist.${COLOR_RESET}"
    return 1
  fi
}

# Function to start the process
start_process() {
  local binary_path="$1"
  echo -e "${COLOR_YELLOW}Starting the application...${COLOR_RESET}"
  "$binary_path" &
  local new_pid=$!
  echo -e "${COLOR_GREEN}Application started with PID $new_pid${COLOR_RESET}"
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
  echo -e "${COLOR_YELLOW}Building binary $binary_name...${COLOR_RESET}"
  go build -o "$binary_name" .
  if [ $? -ne 0 ]; then
    echo -e "${COLOR_RED}Failed to build binary $binary_name.${COLOR_RESET}"
    exit 1
  fi
  echo -e "${COLOR_GREEN}Binary $binary_name built successfully.${COLOR_RESET}"
}

# Function to find an existing binary in the current directory
find_existing_binary() {
  local existing_binaries=($(find . -maxdepth 1 -type f -executable -not -name "*.sh" -not -name "*.env" -not -name "*.go" -not -name "*.mod" -not -name "*.sum"))

  if [ ${#existing_binaries[@]} -gt 0 ]; then
    echo -e "${COLOR_CYAN}Found existing binaries:${COLOR_RESET}"
    local index=1
    local binary_found=0
    for binary in "${existing_binaries[@]}"; do
      echo -e "  ${COLOR_CYAN}$index) ${binary}${COLOR_RESET}"
      if [[ "$(basename "$binary")" == "$BINARY_NAME" ]]; then
        binary_found=1
        echo -e "${COLOR_YELLOW}Binary $BINARY_NAME already exists. Stopping existing process if running...${COLOR_RESET}"
        
        # Find and stop the running process for the existing binary
        local running_pid=$(pgrep -f "$(basename "$binary")")
        if [ -n "$running_pid" ]; then
          stop_process "$running_pid"
        fi
        
        echo -e "${COLOR_YELLOW}Starting the application with existing binary...${COLOR_RESET}"
        # Start the process with the existing binary
        start_process "$binary"
        return
      fi
      index=$((index + 1))
    done

    if [ "$binary_found" -eq 0 ]; then
      read -p "Would you like to use one of these binaries instead of building a new one? (y/n): " use_existing
      if [[ "$use_existing" == "y" || "$use_existing" == "Y" ]]; then
        read -p "Enter the number of the binary you want to use: " chosen_number
        if [[ "$chosen_number" =~ ^[0-9]+$ ]] && [ "$chosen_number" -ge 1 ] && [ "$chosen_number" -le "${#existing_binaries[@]}" ]; then
          local chosen_binary="${existing_binaries[$((chosen_number - 1))]}"
          local binary_name=$(get_binary_name)

          if [[ -n "$binary_name" ]]; then
            echo -e "${COLOR_YELLOW}Renaming existing binary to $binary_name...${COLOR_RESET}"
            mv "$chosen_binary" "./$binary_name"
            echo -e "${COLOR_GREEN}Binary renamed to $binary_name.${COLOR_RESET}"
            echo -e "${COLOR_YELLOW}Updating .env with the new BINARY_NAME...${COLOR_RESET}"

            # Update the .env file with the new binary name
            sed -i "s|^BINARY_NAME=.*|BINARY_NAME=$binary_name|g" "$ENV_FILE"
          else
            echo -e "${COLOR_RED}BINARY_NAME not found in $ENV_FILE. Cannot rename the binary.${COLOR_RESET}"
            exit 1
          fi
        else
          echo -e "${COLOR_RED}Invalid option selected. Exiting.${COLOR_RESET}"
          exit 1
        fi
      fi
    fi
  fi
}

# Main script logic
if [ ! -f "$ENV_FILE" ]; then
  create_env_file
fi

BINARY_NAME=$(get_binary_name)
if [ -z "$BINARY_NAME" ]; then
  echo -e "${COLOR_RED}BINARY_NAME not found in $ENV_FILE.${COLOR_RESET}"
  exit 1
fi

# Find and handle existing binaries before building a new one
find_existing_binary

# Define the path to the binary using BINARY_NAME
BINARY_PATH="./$BINARY_NAME"

# Check if binary exists
if [ ! -f "$BINARY_PATH" ]; then
  echo -e "${COLOR_YELLOW}Binary $BINARY_NAME not found. Building...${COLOR_RESET}"
  build_binary "$BINARY_NAME"
fi

# Get the PID from the .env file
PID=$(get_pid)

# Stop the existing process if running
if [ -n "$PID" ]; then
  stop_process "$PID"
else
  echo -e "${COLOR_YELLOW}No PID found in $ENV_FILE. Starting a new process.${COLOR_RESET}"
fi

# Start the process
start_process "$BINARY_PATH"

echo -e "${COLOR_GREEN}Operation completed.${COLOR_RESET}"
