# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with the API service component in this repository.

## API Development Commands

### Testing API Code
```bash
# Run all API tests
go test ./core/api/...

# Run specific service tests
go test ./core/api/service -run TestObjectService

# Run with verbose output
go test -v ./core/api/server

# Generate/update mocks before testing
make test-deps
```

### API Documentation
```bash
# Generate OpenAPI documentation
make openapi

# Documentation generated in core/api/docs/
# - openapi.yaml
# - openapi.json
```

## Architecture Overview

### Layer Structure

The API follows a clean layered architecture:

1. **Handler Layer** (`/handler/`) - HTTP request handling
   - Parses requests and validates input
   - Calls service methods
   - Maps errors to HTTP status codes
   - Returns JSON responses

2. **Service Layer** (`/service/`) - Business logic
   - Implements core functionality
   - Interacts with middleware via interfaces
   - Handles caching and data transformation
   - No HTTP concerns

3. **Model Layer** (`/model/`) - Data structures
   - Request/response DTOs
   - API-specific data models
   - JSON serialization tags

4. **Core Interfaces** (`/core/`) - External dependencies
   - Defines interfaces for middleware interaction
   - Keeps API decoupled from implementation

### Key Patterns

#### Handler Pattern
```go
func ResourceActionHandler(s *service.Service) gin.HandlerFunc {
    return func(c *gin.Context) {
        // 1. Extract and validate parameters
        var req apimodel.RequestType
        if err := c.ShouldBindJSON(&req); err != nil {
           apiErr := util.CodeToAPIError(http.StatusBadRequest, err.Error())
           c.JSON(http.StatusBadRequest, apiErr)
           return
        }
        
        // 2. Call service layer
        result, err := s.ResourceAction(c, req)
		code := util.MapErrorCode(err, 
			util.ErrToCode(service.ErrMissingAppName, http.StatusBadRequest), 
			util.ErrToCode(service.ErrFailedCreateNewChallenge, http.StatusInternalServerError)
         )
         
         if code != http.StatusOK {
			 apiErr := util.CodeToAPIError(code, err.Error())
			 c.JSON(code, apiErr)
			 return
         }
        
        // 3. Return response
        c.JSON(http.StatusOK,, model.ResponseType{Data: result})
    }
}
```

#### Testing Guidelines

1. **Test Structure**: Use fixture pattern for consistent test setup
   ```go
   type fixture struct {
       service  *Service
       mwMock   *mock_apicore.MockClientCommands
       // other mocks as needed
   }

   func newFixture(t *testing.T) *fixture {
       mwMock := mock_apicore.NewMockClientCommands(t)
       service := NewService(mwMock, gatewayUrl, techSpaceId, crossSpaceSubService)
       return &fixture{
           service: service,
           mwMock:  mwMock,
       }
   }

   func TestServiceMethod(t *testing.T) {
       fx := newFixture(t)
       
       // Setup expectations using testify/mock
       fx.mwMock.On("Method", mock.Anything, expectedArgs).Return(result, nil)
       
       // Execute
       result, err := fx.service.Method(context.Background(), ...)
       
       // Assert
       assert.NoError(t, err)
       assert.Equal(t, expected, result)
   }
   ```

2. **Mock Generation**: Use mockery with `.mockery.yaml` configuration
   ```yaml
   github.com/anyproto/anytype-heart/core/api/core:
    interfaces:
        AccountService:
   ```

3. **Mock Library**: Use `github.com/stretchr/testify/mock` (not gomock)
   ```go
   import "github.com/stretchr/testify/mock"
   
   mockService := mock_filter.NewMockPropertyService(t)
   mockService.On("Method", args...).Return(results...)
   ```

4. **Table-Driven Tests**: Preferred for comprehensive test coverage
   ```go
   tests := []struct {
       name          string
       input         InputType
       setupMock     func(m *MockType)
       expectedError string
       checkResult   func(t *testing.T, result ResultType)
   }{
       // test cases
   }
   
   for _, tt := range tests {
       t.Run(tt.name, func(t *testing.T) {
           // test implementation
       })
   }
   ```

5. **Assertion Style**: Use testify assertions
   ```go
   assert.Equal(t, expected, actual)
   require.NoError(t, err)
   require.Len(t, slice, expectedLen)
   ```

6. **Mock Expectations**: Use `mock.Anything` for flexible matching
   ```go
   mockService.On("Method", 
       mock.Anything,  // for parameters you don't need to match exactly
       specificValue,  // for parameters that must match
   ).Return(result, nil)
   ```

### API Resources

Main resources exposed by the API:

- **Auth** - Authentication endpoints
- **Objects** - Core content objects (pages, notes, etc.)
- **Spaces** - Workspace management
- **Properties** - Object property definitions
- **Types** - Object type management
- **Tags** - Property tag system
- **Templates** - Object templates
- **Lists** - Queries (formerly Sets) and Collections
- **Members** - Space membership
- **Search** - Global and space search

### Middleware Stack

Request processing order:
1. **Recovery** - Panic recovery
2. **Metadata** - API version headers
3. **Logger** - Request logging (debug only)
4. **Pagination** - Query parameter parsing
5. **Cache** - Initialize caches if needed
6. **Auth** - Bearer token validation
7. **RateLimit** - Request throttling
8. **Analytics** - Event tracking
9. **Filters** - Query filtering

### Important Conventions

1. **Authentication**: Bearer token in Authorization header
2. **Pagination**: Use offset/limit query parameters (default: offset=0, limit=100)
3. **Error Handling**: Use `util.MapErrorCode()` and `util.ErrToCode()` for consistent error responses
4. **Response Format**: Paginated responses use `PaginatedResponse[T]` wrapper
5. **Space Scoping**: Most resources are scoped to a space ID in the URL path
6. **Caching**: Types, properties, and tags are cached - use GetCached* methods
7. **Testing**: Always use mocks for middleware dependencies
8. **OpenAPI**: Update Swagger annotations when changing endpoints

### Common Tasks

#### Adding a New Endpoint
1. Define request/response models in `/model/`
2. Add service method in `/service/`
3. Create handler in `/handler/`
4. Add route in `/server/routes.go`
5. Write tests for both service and handler
6. Update OpenAPI annotations
7. Run `make openapi` to regenerate docs

#### Modifying Existing Endpoints
1. Check impact on API compatibility
2. Update models if needed
3. Modify service logic
4. Update tests
5. Regenerate OpenAPI docs

#### Debugging API Issues
1. Enable debug logging in config
2. Check middleware order for request processing
3. Verify authentication token validity
4. Use integration tests for end-to-end validation
5. Check rate limiting configuration