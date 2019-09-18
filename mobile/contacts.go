package mobile

func ListContacts() ([]byte, error) {
	return anytype.Contacts()
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
