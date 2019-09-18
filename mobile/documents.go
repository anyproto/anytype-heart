package mobile

import (
	"github.com/golang/protobuf/proto"
	"github.com/requilence/go-anytype/pb"
	"github.com/requilence/go-anytype/core"
)

func DocumentsTree() ([]byte, error) {
	docs, err := anytype.DocumentsTree()
	if err != nil {
		return nil, err
	}

	if docs == nil {
		docs = []*pb.Document{}
	}

	return proto.Marshal(&pb.DocumentList{Items: docs})
}

func Document(id string) ([]byte, error) {
	doc, err := anytype.DocumentView(id)
	if err != nil {
		return nil, err
	}

	return proto.Marshal(doc)
}

func DocumentVersions(docId string, offset string, limit int) ([]byte, error) {
	doc, err := anytype.Document(docId)
	if err != nil {
		return nil, err
	}

	versions, err := doc.GetVersions(offset, limit, true)
	if err != nil {
		return nil, err
	}

	var versionsPb []*pb.DocumentVersion
	for _, version := range versions {
		versionsPb = append(versionsPb, version.DocumentVersion)
	}

	return proto.Marshal(&pb.DocumentVersionList{Items: versionsPb})
}

func DocumentLastVersion(docId string) ([]byte, error) {
	doc, err := anytype.Document(docId)
	if err != nil {
		return nil, err
	}

	version, err := doc.GetLastVersion()
	if err != nil {
		return nil, err
	}

	return proto.Marshal(version)
}

func DocumentAddVersion(docId string, data []byte) ([]byte, error) {
	var version pb.DocumentVersion
	err := proto.Unmarshal(data, &version)
	if err != nil {
		return nil, err
	}

	doc, err := anytype.Document(docId)
	if err != nil {
		return nil, err
	}

	newVer, err := doc.AddVersion(&core.DocumentVersion{&version},false)
	if err != nil {
		return nil, err
	}

	return proto.Marshal(newVer)
}

func AddDocument(data []byte) ([]byte, error) {
	var docConfig pb.AddDocumentConfig
	err := proto.Unmarshal(data, &docConfig)
	if err != nil {
		return nil, err
	}

	newDoc, err := anytype.AddDocument(docConfig)
	if err != nil {
		return nil, err
	}

	return proto.Marshal(newDoc)
}

/*
import (
	"net/http"
	"path/filepath"
	tmobile "github.com/textileio/go-textile/mobile"
	core "github.com/textileio/go-textile/core"

)

func (a *Anytype) DocumentsTree() ([]byte, error) {
	if !a.Node.Started(){
		return nil, core.ErrStopped
	}
	var threadById = make(map[string]*core.Thread)
	var hasParent = make(map[string]struct{})

	accountThread := a.Node.ThreadByKey(a.Node.config.Account.Address)

	for _, th := range a.Node.Threads() {
		threadById[th.Id] = th
		for _, child := range th.ChildrenIds {
			hasParent[child] = struct{}{}
		}
	}

	// temp dirty way to workaround merging non-atomic account thread children list changes
	var rootThreads = accountThread.ChildrenIds

	tree := make([]*Document, 0)
	for _, childId := range rootThreads {
		child := threadById[childId]
		if child == nil {
			log.Errorf("thread child not found: %s", childId)
			continue
		}

		doc, err := child.ToDocument()
		if err != nil {
			// ignore error because this is account or template thread
			continue
		}

		doc.Children = doc.childrenTree()
		tree = append(tree, doc)
	}

	templatesThread := a.Node.ThreadByKey("templates")
	if templatesThread == nil {
		g.JSON(http.StatusOK, tree)
		return
	}

	for _, childId := range templatesThread.ChildrenIds {
		child := threadById[childId]
		if child == nil {
			log.Errorf("thread child not found: %s", childId)
			continue
		}

		doc, err := child.ToDocument()
		if err != nil {
			log.Errorf("failed to create doc from thread: %s", err.Error())
			continue
		}

		doc.Template = true
		doc.Children = doc.childrenTree()
		tree = append(tree, doc)
	}

}

*/
