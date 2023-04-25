package indexer

import "fmt"

type reindexFlags struct {
	bundledTypes            bool
	removeAllIndexedObjects bool
	bundledRelations        bool
	eraseIndexes            bool
	threadObjects           bool
	fileObjects             bool
	fulltext                bool
	bundledTemplates        bool
	bundledObjects          bool
	fileKeys                bool
}

func (f *reindexFlags) any() bool {
	return f.bundledTypes ||
		f.removeAllIndexedObjects ||
		f.bundledRelations ||
		f.eraseIndexes ||
		f.threadObjects ||
		f.fileObjects ||
		f.fulltext ||
		f.bundledTemplates ||
		f.bundledObjects ||
		f.fileKeys
}

func (f *reindexFlags) enableAll() {
	f.bundledTypes = true
	f.removeAllIndexedObjects = true
	f.bundledRelations = true
	f.eraseIndexes = true
	f.threadObjects = true
	f.fileObjects = true
	f.fulltext = true
	f.bundledTemplates = true
	f.bundledObjects = true
	f.fileKeys = true
}

func (f *reindexFlags) String() string {
	return fmt.Sprintf("%#v", f)
}
