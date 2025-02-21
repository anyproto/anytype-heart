package indexer

import "fmt"

type reindexFlags struct {
	bundledTypes            bool
	removeAllIndexedObjects bool
	bundledRelations        bool
	objects                 bool
	fileObjects             bool
	bundledTemplates        bool
	bundledObjects          bool
	fileKeys                bool
	removeOldFiles          bool
	deletedObjects          bool
	eraseLinks              bool
	removeParticipants      bool
}

func (f *reindexFlags) any() bool {
	return f.bundledTypes ||
		f.removeAllIndexedObjects ||
		f.bundledRelations ||
		f.objects ||
		f.fileObjects ||
		f.bundledTemplates ||
		f.bundledObjects ||
		f.fileKeys ||
		f.removeOldFiles ||
		f.deletedObjects ||
		f.removeParticipants ||
		f.eraseLinks
}

func (f *reindexFlags) enableAll() {
	f.bundledTypes = true
	f.removeAllIndexedObjects = true
	f.bundledRelations = true
	f.objects = true
	f.fileObjects = true
	f.bundledTemplates = true
	f.bundledObjects = true
	f.fileKeys = true
	f.removeOldFiles = true
	f.deletedObjects = true
	f.removeParticipants = true
	f.eraseLinks = true
}

func (f *reindexFlags) String() string {
	return fmt.Sprintf("%#v", f)
}
