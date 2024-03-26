package indexer

import "fmt"

type reindexFlags struct {
	bundledTypes            bool
	removeAllIndexedObjects bool
	bundledRelations        bool
	objects                 bool
	fileObjects             bool
	fulltext                bool
	fulltextErase           bool
	bundledTemplates        bool
	bundledObjects          bool
	fileKeys                bool
	removeOldFiles          bool
}

func (f *reindexFlags) any() bool {
	return f.bundledTypes ||
		f.removeAllIndexedObjects ||
		f.bundledRelations ||
		f.objects ||
		f.fileObjects ||
		f.fulltext ||
		f.fulltextErase ||
		f.bundledTemplates ||
		f.bundledObjects ||
		f.fileKeys ||
		f.removeOldFiles
}

func (f *reindexFlags) enableAll() {
	f.bundledTypes = true
	f.removeAllIndexedObjects = true
	f.bundledRelations = true
	f.objects = true
	f.fileObjects = true
	f.fulltext = true
	f.fulltextErase = true
	f.bundledTemplates = true
	f.bundledObjects = true
	f.fileKeys = true
	f.removeOldFiles = true
}

func (f *reindexFlags) String() string {
	return fmt.Sprintf("%#v", f)
}
