//go:generate mockgen -package testMock -destination anytype_mock.go github.com/anytypeio/go-anytype-middleware/core/anytype Service,SmartBlock,File,Image
//go:generate mockgen -package testMock -destination history_mock.go github.com/anytypeio/go-anytype-middleware/core/block/history History
package testMock
