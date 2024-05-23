package objectstore

import "github.com/anyproto/anytype-heart/util/badgerhelper"

func (s *dsObjectStore) SetActiveView(objectId, blockId, viewId string) error {
	return badgerhelper.SetValue(s.db, pagesActiveViewBase.ChildString(objectId).ChildString(blockId).Bytes(), viewId)
}

func (s *dsObjectStore) GetActiveView(objectId, blockId string) (string, error) {
	return badgerhelper.GetValue(s.db, pagesActiveViewBase.ChildString(objectId).ChildString(blockId).Bytes(), bytesToString)
}
