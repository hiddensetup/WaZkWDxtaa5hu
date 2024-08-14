# Use the official Ubuntu base image
FROM ubuntu:latest

# Set environment variables for Go installation
ENV GOROOT /usr/local/go
ENV GOPATH /go
ENV PATH $GOPATH/bin:$GOROOT/bin:$PATH

# Install required dependencies
RUN apt-get update && \
    apt-get install -y wget

# Download and install Go 1.20
RUN wget https://golang.org/dl/go1.20.linux-amd64.tar.gz && \
    tar -C /usr/local -xzf go1.20.linux-amd64.tar.gz && \
    rm go1.20.linux-amd64.tar.gz

# Verify Go installation
RUN go version

# Your additional instructions go here...

# Set the working directory (example)
WORKDIR /app

# Copy your application files (example)
# COPY . /app

# Set the entry point (example)
# CMD ["./your-app-binary"]

# Expose any necessary ports (example)
# EXPOSE 8080

# Build your Docker image with:
# docker build -t your-image-name .
