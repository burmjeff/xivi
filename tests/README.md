# Testing Structure

This directory contains all tests for the Xivi application. The tests are organized into different categories to make them easier to maintain and run.

## Directory Structure

```
tests/
├── unit/                  # Unit tests for individual components
│   ├── backend/           # Backend unit tests
│   │   ├── pkg/           # Tests for utility packages
│   │   ├── app/           # Tests for application logic
│   │   └── platform/      # Tests for platform-specific code
│   └── frontend/          # Frontend unit tests (future)
├── integration/           # Integration tests between components
├── e2e/                   # End-to-end tests
└── fixtures/              # Test fixtures and mock data
```

## Test Types

### Unit Tests

Unit tests focus on testing individual functions or methods in isolation. They should be fast, reliable, and not depend on external services like databases or APIs.

Example:
```go
func TestUpdateDynamicGroup(t *testing.T) {
    // Test cases
    testCases := []struct {
        name        string
        group       models.TemplateGroup
        description string
    }{
        // Test cases here
    }

    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            // Test logic here
        })
    }
}
```

### Integration Tests

Integration tests verify that different components work together correctly. These tests may interact with databases or other services.

### End-to-End Tests

E2E tests simulate real user scenarios and test the entire application flow from start to finish.

## Running Tests

### Running All Tests

```bash
go test ./tests/...
```

### Running Unit Tests Only

```bash
go test ./tests/unit/...
```

### Running Tests for a Specific Package

```bash
go test ./tests/unit/backend/pkg/utils/...
```

## Writing Tests

### Best Practices

1. **Use table-driven tests**: Define multiple test cases in a slice and iterate through them.
2. **Mock external dependencies**: Use interfaces and mock implementations for database access, etc.
3. **Test edge cases**: Include tests for error conditions and boundary values.
4. **Keep tests independent**: Each test should be able to run independently of others.
5. **Use descriptive test names**: Name tests clearly to indicate what they're testing.

### Example Test Structure

```go
func TestSomething(t *testing.T) {
    // Setup
    // ...

    // Test cases
    testCases := []struct {
        name     string
        input    SomeType
        expected SomeType
        wantErr  bool
    }{
        // Test cases here
    }

    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            // Execute the function being tested
            result, err := FunctionBeingTested(tc.input)

            // Check for expected errors
            if tc.wantErr {
                if err == nil {
                    t.Errorf("Expected error but got none")
                }
                return
            }

            // Check the result
            if result != tc.expected {
                t.Errorf("Expected %v but got %v", tc.expected, result)
            }
        })
    }
}
```

## Mocking

For tests that require database access or other external dependencies, use mocks to isolate the code being tested.

Example of a mock database implementation:

```go
type MockDB struct {
    // Mock implementations of database methods
}

func (m *MockDB) GetTmplGroup(id int64) (*models.TemplateGroup, error) {
    // Return predefined test data
    return &models.TemplateGroup{
        ID:      id,
        Name:    "Test Group",
        Dynamic: true,
    }, nil
}
```

## Continuous Integration

Tests are automatically run as part of the CI/CD pipeline. All tests must pass before code can be merged into the main branch.
