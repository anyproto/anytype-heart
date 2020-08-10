//go:generate mockgen -package testMock -destination anytype_mock.go github.com/anytypeio/go-anytype-middleware/core/anytype Service,SmartBlock,SmartBlockSnapshot,File,Image
//go:generate mockgen -package testMock -destination history_mock.go github.com/anytypeio/go-anytype-middleware/core/block/undo History
package testMock
