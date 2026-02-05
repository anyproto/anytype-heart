# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with this repository.

## Commit Messages

Every commit message must be prefixed with an issue number following this pattern:
```
GO-{issueNumber} Short description

Optional longer description.
```

Example:
```
GO-6443 Add chat name to push notifications

Include chatName field in push notification payload so users can see
which chat a message was sent from.
```

## Testing Guidelines

### Test Structure

1. **Use fixture pattern** for consistent test setup:
   ```go
   type fixture struct {
       *service
       objectStore          *objectstore.StoreFixture
       // other dependencies
   }

   func newFixture(t *testing.T) *fixture {
       objectStore := objectstore.NewStoreFixture(t)
       // setup mocks and dependencies
       return &fixture{
           service:     New().(*service),
           objectStore: objectStore,
       }
   }
   ```

2. **Use `want` structure** in tests for clarity - it makes expected values explicit:
   ```go
   t.Run("test case name", func(t *testing.T) {
       // given
       fx := newFixture(t)
       req := SomeRequest{...}
       want := &ExpectedType{
           Field1: "expected value",
           Field2: 123,
       }

       // when
       got, err := fx.methodUnderTest(req)

       // then
       require.NoError(t, err)
       assert.Equal(t, want, got)
   })
   ```

3. **Use `AddObjects` method** of `StoreFixture` for setting up test data:
   ```go
   fx.objectStore.AddObjects(t, spaceId, []objectstore.TestObject{
       {
           bundle.RelationKeyId:             domain.String("objectId"),
           bundle.RelationKeyName:           domain.String("Object Name"),
           bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
       },
   })
   ```

4. **Tech space ID** - use `objectstore.TestTechSpaceId` constant for space view setup:
   ```go
   fx.objectStore.AddObjects(t, objectstore.TestTechSpaceId, []objectstore.TestObject{
       {
           bundle.RelationKeyId:             domain.String("spaceView1"),
           bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_spaceView)),
           bundle.RelationKeyTargetSpaceId:  domain.String(spaceId),
           bundle.RelationKeyName:           domain.String(spaceName),
       },
   })
   ```

5. **Test naming**: Use descriptive names that explain what's being tested:
   - `"basic message with space and sender names"`
   - `"message with attachments"`
   - `"message with emoji marks"`
   - `"empty chat name"`

### Assertion Style

Use testify assertions:
```go
assert.Equal(t, expected, actual)
require.NoError(t, err)
require.Len(t, slice, expectedLen)
```

### Mock Usage

1. **Mock generation**: Use mockery with `.mockery.yaml` configuration
   ```bash
   # Regenerate mocks
   make test-deps
   ```

2. **Mock expectations**: Use `EXPECT()` pattern with testify/mock:
   ```go
   fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{
       Records: []*domain.Details{},
   }, nil).Maybe()
   ```

3. **Flexible matching**: Use `mock.Anything` for parameters you don't need to match exactly
