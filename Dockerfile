# Use a more specific golang version since 1.23 doesn't exist yet
FROM golang:1.23-alpine

# Install required system dependencies
RUN apk add --no-cache gcc musl-dev postgresql-dev

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the entire project
COPY . .

# Build the application
RUN go build -o scrapper ./cmd/scrapper/main.go

# Expose port (if your API needs it)
EXPOSE 8000

# Command to run the application
CMD ["./scrapper"]