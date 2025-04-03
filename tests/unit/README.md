# Unit Tests

This directory contains unit tests for individual components of the Xivi application.

## Structure

The unit tests follow the same package structure as the main application code:

```
unit/
├── backend/           # Backend unit tests
│   ├── pkg/           # Tests for utility packages
│   │   ├── utils/     # Tests for utility functions
│   │   └── ...
│   ├── app/           # Tests for application logic
│   │   ├── controllers/ # Tests for controllers
│   │   ├── models/    # Tests for models
│   │   ├── queries/   # Tests for database queries
│   │   └── ...
│   └── platform/      # Tests for platform-specific code
│       ├── cron/      # Tests for cron jobs
│       ├── database/  # Tests for database connections
│       └── ...
└── frontend/          # Frontend unit tests (future)
```

## Writing Unit Tests

Unit tests should:

1. Test a single function or method in isolation
2. Mock any external dependencies
3. Be fast and reliable
4. Cover both success and error cases

## Example

Here's an example of a unit test for the `UpdateDynamicGroup` function:

```go
func TestUpdateDynamicGroup(t *testing.T) {
    // Test cases
    testCases := []struct {
        name        string
        group       models.TemplateGroup
        description string
    }{
        {
            name: "Group is not dynamic",
            group: models.TemplateGroup{
                ID:           1,
                Name:         "Test Group",
                Dynamic:      false,
                DynamicGroup: func() *int64 { id := int64(123); return &id }(),
            },
            description: "Should return early without error when group is not dynamic",
        },
        // More test cases...
    }

    // Run test cases
    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            // Execute the function
            utils.UpdateDynamicGroup(tc.group)
            // Assertions...
        })
    }
}
```

## Running Unit Tests

To run all unit tests:

```bash
go test ./tests/unit/...
```

To run tests for a specific package:

```bash
go test ./tests/unit/backend/pkg/utils/...
```

To run a specific test:

```bash
go test ./tests/unit/backend/pkg/utils/... -run TestUpdateDynamicGroup
```

## Test Coverage

To generate a test coverage report:

```bash
go test ./tests/unit/... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

This will open a browser window showing which lines of code are covered by tests.
