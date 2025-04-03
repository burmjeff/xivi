#!/bin/bash

# Run all tests
echo "Running all tests..."
go test ./tests/...

# Run unit tests only
echo "Running unit tests only..."
go test ./tests/unit/...

# Run tests for a specific package
echo "Running tests for playlist_tools package..."
go test ./tests/unit/backend/pkg/utils/...

# Generate test coverage report
echo "Generating test coverage report..."
go test ./tests/... -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
echo "Coverage report generated at coverage.html"
