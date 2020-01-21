//go:generate mockgen -package testMock -destination anytype_mock.go github.com/anytypeio/go-anytype-middleware/core/anytype Anytype,Block,BlockVersion,BlockVersionMeta,File,Image
package testMock
