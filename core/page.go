package core

import (
	"errors"
	"fmt"
	"github.com/gogo/protobuf/proto"
	structpb "github.com/golang/protobuf/ptypes/struct"

	tcore "github.com/textileio/go-textile/core"
	mill2 "github.com/textileio/go-textile/mill"
	tpb "github.com/textileio/go-textile/pb"
)

const (
	mergeFileCaption = "Merge"
	defaultDocName   = "Untitled"
	smartBlockSchema = "QmTShDQr2PeWEXE5D8r77Lz5NeyLK7NNRENXtQHTvqo9F5"
)

var errorNotFound = fmt.Errorf("not found")

type Page struct {
	thread *tcore.Thread `json:",inline"`
	node   *Anytype
}

func (page *Page) GetThread() *tcore.Thread {
	return page.thread
}

func (page *Page) GetId() string {
	return page.thread.Id
}

func (page *Page) GetType() BlockType {
	return BlockType_PAGE
}

func (page *Page) GetVersion(id string) (SmartBlockVersion, error) {
	files, err := page.node.Textile.Node().File(id)
	if err != nil {
		return nil, err
	}

	if len(files.Files) == 0 {
		return nil, errors.New("version block not found")
	}

	block := &Block{}

	plaintext, err := readFile(page.node.Textile.Node(), files.Files[0].File)
	if err != nil {
		return nil, fmt.Errorf("readFile error: %s", err.Error())
	}

	err = proto.Unmarshal(plaintext, block)
	if err != nil {
		return nil, fmt.Errorf("page version proto unmarshal error: %s", err.Error())
	}

	version := &PageVersion{VersionId: files.Block, PageId: page.GetId(), Date: files.Date, User: files.User.Address, Content: block.GetPage()}

	return version, nil
}

func (page *Page) GetLastVersion() (SmartBlockVersion, error) {
	versions, err := page.GetVersions("", 1, false)
	if err != nil {
		return nil, err
	}

	if len(versions) == 0 {
		return nil, errorNotFound
	}

	return versions[0], nil
}

func (page *Page) GetVersions(offset string, limit int, metaOnly bool) ([]SmartBlockVersion, error) {
	files, err := page.node.Textile.Node().Files(offset, limit, page.thread.Id)
	if err != nil {
		return nil, err
	}

	var versions []SmartBlockVersion
	if len(files.Items) == 0 {
		return versions, nil
	}

	for _, item := range files.Items {
		version := &PageVersion{VersionId: item.Block, PageId: page.GetId(), Date: item.Date, User: item.User.Address}

		if metaOnly {
			versions = append(versions, version)
			continue
		}

		block := &Block{}

		plaintext, err := readFile(page.node.Textile.Node(), item.Files[0].File)
		if err != nil {
			return nil, fmt.Errorf("readFile error: %s", err.Error())
		}

		err = proto.Unmarshal(plaintext, block)
		if err != nil {
			return nil, fmt.Errorf("page version proto unmarshal error: %s", err.Error())
		}

		version.Content = block.GetPage()
		versions = append(versions, version)
	}

	return versions, nil
}

/*func trimEmptyTrailingBlocks(page *PageVersion) *PageVersion {
	var lastNotEmptyBlock int
	for i := len(page.Blocks) - 1; i >= 0; i-- {
		block := page.Blocks[i]
		if _, isText := block.Content.(*Block_Text); !isText {
			lastNotEmptyBlock = i
			break
		}

		if len(block.GetText().Text) > 0 {
			lastNotEmptyBlock = i
			break
		}
	}

	page.Blocks = page.Blocks[0 : lastNotEmptyBlock+1]
	return page
}

/*
func isVersionsEqual(version1, version2 *PageVersion) bool {
	version1JSON, err := json.Marshal(version1.Blocks)
	if err != nil {
		log.Errorf("version marshal error: %s", err.Error())
		return false
	}

	version2JSON, err := json.Marshal(version2.Blocks)
	if err != nil {
		log.Errorf("version marshal error: %s", err.Error())
		return false
	}

	return bytes.Equal(version1JSON, version2JSON)
}*/

func (page *Page) AddVersion(newVersionInterface SmartBlockVersion) error {
	lastVersion, err := page.GetLastVersion()

	var newVersion *PageVersion
	var ok bool
	if newVersion, ok = newVersionInterface.(*PageVersion); !ok {
		return fmt.Errorf("unxpected smartblock type")
	}

	lastVersionB, _ := proto.Marshal(lastVersion.(*PageVersion).Content.Blocks)
	newVersionB, _ := proto.Marshal(newVersion.Content.Blocks)
	if string(lastVersionB) == string(newVersionB) {
		log.Debugf("[MERGE] new version has the same blocks as the last version - ignore it")
		// do not insert the new version if no blocks have changed
		newVersion.VersionId = lastVersion.GetVersionId()
		newVersion.User = lastVersion.GetUser()
		newVersion.Date = lastVersion.GetDate()
	} else {
		fmt.Printf("version differs:new %s\n%s\n\n---\n\nlast %s\n%s", newVersion.VersionId, string(newVersionB), lastVersion.GetVersionId(), string(lastVersionB))
	}
	//	}

	if newVersion.VersionId != "" {
		/*	err = page.Modify(page.ChildrenIds, newVersion.Name, newVersion.Icon)
			if err != nil {
				return nil, err
			}*/
		return nil
	}

	newVersionB, err = proto.Marshal(newVersion.Content)
	if err != nil {
		return err
	}

	mill := &mill2.Json{}

	conf := tcore.AddFileConfig{
		Media:     "application/json",
		Plaintext: false,
		Input:     newVersionB,
		//Gzip:      true,
	}

	/*if isMerge && newVersion.Date != nil {
		conf.Added = util.ProtoTs(newVersion.Date.UnixNano())
	}*/

	newFile, err := page.node.Textile.Node().AddFileIndex(mill, conf)
	if err != nil {
		return fmt.Errorf("AddFileIndex error: %s", err.Error())
	}

	//log.Debugf("(%p) AddFileIndex %s",  page.textile, spew.Sdump(newFile))

	node, keys, err := page.node.Textile.Node().AddNodeFromFiles([]*tpb.FileIndex{newFile})
	if err != nil {
		return fmt.Errorf("AddNodeFromFiles error: %s", err.Error())
	}

	//log.Debugf("AddNodeFromFiles %s %s %s", spew.Sdump(node.Cid()), spew.Sdump(keys.Files), spew.Sdump(node.Links()))

	var caption = newVersion.GetName()

	block, err := page.thread.AddFiles(node, "version", caption, keys.Files)
	if err != nil {
		return fmt.Errorf("thread.AddFiles error: %s", err.Error())
	}
	//log.Debugf("(%p) Thread.AddFiles %s",  page.textile, block.B58String())

	newVersion.VersionId = block.B58String()
	//fmt.Printf("saved new version %s... parent %s\n", newVersion.Id, newVersion.p)

	newVersion.User = page.node.Textile.Node().Account().Address()
	newBlock, err := page.node.Textile.Node().Block(block.B58String())
	if err != nil {
		log.Errorf("failed to get the block %s: %s", newBlock.Id, err.Error())
	}

	if newBlock != nil {
		newVersion.Date = newBlock.Date
	}

	return err
}

func (page *Page) GetExternalFields() *structpb.Struct {
	ver, err := page.GetLastVersion()
	if err != nil {
		return nil
	}

	return ver.GetExternalFields()
}

/*func (page *Page) Modify(childrenIds []string, name string, icon string) error {
	var childrensStr string
	removedChild, addedChild := util.DiffStringSlice(page.ChildrenIds, childrenIds)
	log.Debugf("page modify: removed(%v) added(%v)", removedChild, addedChild)
	if len(removedChild) > 0 || len(addedChild) > 0 {
		if len(childrenIds) == 0 {
			childrensStr = "-"
		} else {
			childrensStr = strings.Join(childrenIds, ",")
		}
	}

	var newIconName string
	if name != page.Name || icon != page.Icon {
		newIconName = icon + pageIconAndNameSeperator + name
	}

	if newIconName == "" && childrensStr == "" {
		return nil
	}
	peers := page.Peers()

	if len(addedChild) > 0 {
		isThreadChildren := make(map[string]struct{})
		for _, thrdId := range addedChild {
			isThreadChildren[thrdId] = struct{}{}
		}

		for _, childThread := range page.node.Textile.Node().Threads() {
			if _, isChildThread := isThreadChildren[childThread.Id]; !isChildThread {
				continue
			}

			var childHasPeer = make(map[string]struct{})
			for _, peer := range childThread.Peers() {
				childHasPeer[peer.Id] = struct{}{}
			}

			for _, tpeer := range peers {
				if _, isChildHasThisParentPeer := childHasPeer[tpeer.Id]; isChildHasThisParentPeer {
					continue
				}

				go func(thread *tcore.Thread, peerID string) {
					peer := page.node.Textile.Node().Datastore().Peers().Get(peerID)
					err := page.node.Textile.Node().AddInvite(page.node.Textile.Node().Threads(), peer.Address)
					if err != nil {
						log.Errorf("childThread.AddInvite(%s) error: %s", peerID, err.Error())
						return
					}
				}(childThread, tpeer.Id)

				childHasPeer[tpeer.Id] = struct{}{}
			}
		}
	}

/*	_, err := page.Thread.(childrensStr, newIconName)
	for _, childrenId := range childrenIds {
		// it is ok to call on thread that not archived because this method has a check inside
		err = page.textile.ArchiveThread(childrenId, false)
		if err != nil {
			log.Errorf("failed to unarchive: %s", err.Error())
		}
	}
	if err != nil {
		return err
	}


	return page.textile.archiveIfAbandoned(removedChild...)

return nil
}*/

/*func  refreshChildren(pages []*pb.Page)  {
	children := d.LastVersion.ChildrenIds()
	for _, childDocId := range children {
		childDoc, err := d.node.Page(childDocId)
		if err != nil {
			log.Errorf("failed to convert child thread to page: %s", err.Error())
			continue
		}
		childDoc.refreshChildren()
		d.Children = append(d.Children, childDoc)
	}
}*/

/*func (t *Page) childrenTree() []*Page {
	return t.childrenTreeIgnoreBranch("")
}*/

/*func (page *Page) childrenTreeIgnoreBranch(ignoreThreadBranch string) []*Page {
	var threadById = make(map[string]*Thread)
	for _, th := range page.node.Textile.Node().loadedThreads {
		threadById[th.Id] = th
	}

	threadById["root"] = page.node.Textile.Node().AccountThread()

	var getChildren func([]string, []string, bool) []*Page
	getChildren = func(breadcrumbs []string, rootIds []string, isTemplate bool) []*Page {
		var tree []*Page
		var breadcrumbsMap = map[string]struct{}{}
		for _, parentID := range breadcrumbs {
			breadcrumbsMap[parentID] = struct{}{}
		}

		for _, id := range rootIds {
			t := threadById[id]

			if t == nil {
				log.Errorf("can't find thread %s", id)
				continue
			}

			if t.Id == ignoreThreadBranch {
				continue
			}

			pageT, err := t.ToPage()
			if err != nil {
				// ignore error because this is account or template thread
				continue
			}

			pageT.Template = isTemplate
			if t.Key == "templates" {
				pageT.Template = true
			}

			// do not traverse the tree when the thread's child is also some of the thread's parents
			if _, parentExistsInBreadcrumbs := breadcrumbsMap[id]; !parentExistsInBreadcrumbs && len(t.ChildrenIds) > 0 {
				pageT.ChildrenIds = getChildren(append(breadcrumbs, id), t.ChildrenIds, pageT.Template)
			}

			tree = append(tree, pageT)
		}
		return tree
	}

	if page == nil {
		return nil
	}

	tree := getChildren([]string{}, []string{page.Id}, false)

	if len(tree) == 0 {
		return nil
	}

	return tree[0].ChildrenIds
}*/
