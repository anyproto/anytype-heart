package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/anytypeio/go-anytype-library/pb"
	"github.com/anytypeio/go-anytype-library/util"
	tcore "github.com/textileio/go-textile/core"
	mill2 "github.com/textileio/go-textile/mill"
	tpb "github.com/textileio/go-textile/pb"
)

const (
	mergeFileCaption = "Merge"
	defaultDocName   = "Untitled"
	documentSchema   = "QmTShDQr2PeWEXE5D8r77Lz5NeyLK7NNRENXtQHTvqo9F5"
)

var errorNotFound = fmt.Errorf("not found")

type DocumentVersion struct {
	*pb.DocumentVersion
}

type DocumentVersions []DocumentVersion

type Document struct {
	*pb.Document

	// todo: refactor in order to remove
	thread *tcore.Thread `json:",inline"`
	node   *Anytype
}

type Documents []Document

type DocumentWithVersion struct {
	*Document        `json:",inline"`
	*DocumentVersion `json:",inline"`
}

type DocumentBlocks struct {
	Blocks []*pb.DocumentBlock `json:"blocks"`
}

type DocumentBlock struct {
	*pb.DocumentBlock
	parentId string
}

type DocumentAddBlock struct {
	ParentId    string `json:"parentId"`
	Position    string `json:"parentId"`
	PositionBy  string `json:"parentId"`
	Type        int    `json:"type"`
	ContentType string `json:"contentType"`
	Content     string `json:"content"`
	Width       int    `json:"width"`
}

type DocumentRemoveBlock struct {
	Id string `json:"id"`
}

type DocumentBlockTypeDocument struct {
	Id string `json:"id"`
}

func (doc *Document) AddChild(childId string, addDocumentBlock bool) error {
	//	err := doc.Thread.AddChild(childId)
	//	if err != nil {
	//		return err
	//	}

	// todo: add block to the document
	return nil
}

func (doc *Document) GetVersion(id string) (*DocumentVersion, error) {
	files, err := doc.node.Textile.Node().File(id)
	if err != nil {
		return nil, err
	}

	if len(files.Files) == 0 {
		return nil, errors.New("version block not found")
	}

	version := &DocumentVersion{}

	icon, name := getIconAndNameFromPackedThreadName(files.Caption)
	if icon != "" && name != "" {
		version.Name = name
		version.Icon = icon
	}

	plaintext, err := readFile(doc.node.Textile.Node(), files.Files[0].File)
	if err != nil {
		return nil, fmt.Errorf("readFile error: %s", err.Error())
	}

	// uncompress if compressed
	uncompressedB, err := util.GzipUncompress(plaintext)
	if err == nil {
		plaintext = uncompressedB
	}

	err = json.Unmarshal(plaintext, &version)
	if err != nil {
		return nil, fmt.Errorf("doc version unmarshal error: %s", err.Error())
	}

	version.Date = files.Date
	version.Id = files.Block
	version.User = files.User.Address

	return version, nil
}

func (doc *Document) GetLastVersion() (*DocumentVersion, error) {
	versions, err := doc.GetVersions("", 1, false)
	if err != nil {
		return nil, err
	}

	if len(versions) == 0 {
		return nil, errorNotFound
	}

	return versions[0], nil
}

func (doc *Document) GetVersions(offset string, limit int, metaOnly bool) ([]*DocumentVersion, error) {
	files, err := doc.node.Textile.Node().Files(offset, limit, doc.Id)
	if err != nil {
		return nil, err
	}

	var versions []*DocumentVersion
	if len(files.Items) == 0 {
		return versions, nil
	}

	for _, item := range files.Items {
		version := &DocumentVersion{&pb.DocumentVersion{}}

		version.Id = item.Block
		version.User = item.User.Address
		version.Date = item.Date

		if metaOnly {
			versions = append(versions, version)
			continue
		}

		icon, name := getIconAndNameFromPackedThreadName(item.Caption)
		if icon != "" && name != "" {
			version.Name = name
			version.Icon = icon
		}

		plaintext, err := readFile(doc.node.Textile.Node(), item.Files[0].File)
		if err != nil {
			return nil, fmt.Errorf("readFile error: %s", err.Error())
		}

		// uncompress if compressed
		uncompressedB, err := util.GzipUncompress(plaintext)
		if err == nil {
			plaintext = uncompressedB
		}

		err = json.Unmarshal(plaintext, &version)
		if err != nil {
			log.Errorf("failed to unmarshal document %s(%s block): %s", doc.Id, item.Block, err.Error())
			continue
		}

		versions = append(versions, version)
	}

	return versions, nil
}

func trimEmptyTrailingBlocks(doc *DocumentVersion) *DocumentVersion {
	var lastNotEmptyBlock int
	for i := len(doc.Blocks) - 1; i >= 0; i-- {
		block := doc.Blocks[i]
		if block.Type != pb.DocumentBlockType_EDITABLE {
			lastNotEmptyBlock = i
			break
		}

		if !strings.Contains(block.Content, `"text":""`) {
			lastNotEmptyBlock = i
			break
		}
	}

	doc.Blocks = doc.Blocks[0 : lastNotEmptyBlock+1]
	return doc
}

func isVersionsEqual(version1, version2 *DocumentVersion) bool {
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
}

func (doc *Document) ChildrenIds() ([]string, error) {
	ver, err := doc.GetLastVersion()
	if err != nil {
		return nil, err
	}

	return ver.ChildrenIds(), nil
}

func (doc *Document) AddVersion(newVersion *DocumentVersion, isMerge bool) (*DocumentVersion, error) {
	newVersion = trimEmptyTrailingBlocks(newVersion)

	/*if len(newVersion.Parents) == 1 {
		parentVersion, err := doc.GetVersion(newVersion.Parents[0])
		if err != nil {
			return nil, fmt.Errorf("can't load parent version: %s", err.Error())
		}
		// if we doesn't make any changes to the provided parent - set the ID to ignore this change
		if isVersionsEqual(parentVersion, newVersion) {
			newVersion.Id = parentVersion.Id
			newVersion.User = parentVersion.User
			newVersion.Date = parentVersion.Date
			newVersion.Parents = parentVersion.Parents
		}
	}*/

	lastVersions, err := doc.GetVersions("", 1, false)
	if newVersion.Id == "" && err == nil && len(lastVersions) > 0 && len(newVersion.Parents) > 0 {
		parentFound := false
		lastVersion := lastVersions[0]
		for _, parent := range newVersion.Parents {
			if parent == lastVersion.Id {
				parentFound = true
				break
			}
		}

		lastVersionB, _ := json.Marshal(lastVersion.Blocks)

		if !parentFound {
			// so we received the new version that was applied to the outdated parent
			// we need to found the ancestor and merge the changes
			/*		ancestorBlockID := doc.Thread.FollowParentsForTheFirstAncestor(newVersion.Parents[0], lastVersion.Id)
					log.Debugf("ancestorBlockID for %s and %s is %s", newVersion.Parents[0], lastVersion.Id, ancestorBlockID.B58String())

					ancestorFileBlock := doc.Thread.FollowParentsUntilBlock([]string{ancestorBlockID.B58String()}, pb.Block_FILES)
					if ancestorFileBlock == nil {
						return nil, fmt.Errorf("can't find ancestor file block")
					}

					log.Debugf("(%p) [MERGE] outdated parent version (parent = %s, last = %s) – need a merge. Found ancestor: %s -> closest FILE parent %s", doc.textile, newVersion.Parents[0], lastVersions[0].Id, ancestorBlockID.B58String(), ancestorFileBlock.Id)

					ancestorVersion, err := doc.GetVersion(ancestorFileBlock.)
					if err != nil {
						return nil, err
					}

					newVersion, err = mergeVersions(ancestorVersion, lastVersion, newVersion)
					if err != nil {
						return nil, err
					}*/
		}

		newVersionB, _ := json.Marshal(newVersion.Blocks)
		if string(lastVersionB) == string(newVersionB) {
			log.Debugf("[MERGE] new version has the same blocks as the last version - ignore it")
			// do not insert the new version if no blocks have changed
			newVersion.Id = lastVersions[0].Id
			newVersion.User = lastVersions[0].User
			newVersion.Date = lastVersions[0].Date
			newVersion.Parents = lastVersions[0].Parents
		} else {
			fmt.Printf("version differs:new %s\n%s\n\n---\n\nlast %s\n%s", newVersion.Id, string(newVersionB), lastVersion.Id, string(lastVersionB))
		}
	}

	if newVersion.Id != "" {
		/*	err = doc.Modify(doc.ChildrenIds, newVersion.Name, newVersion.Icon)
			if err != nil {
				return nil, err
			}*/
		return newVersion, nil
	}

	newVersionB, err := json.Marshal(newVersion)
	if err != nil {
		return nil, err
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

	newFile, err := doc.node.Textile.Node().AddFileIndex(mill, conf)
	if err != nil {
		return nil, fmt.Errorf("AddFileIndex error: %s", err.Error())
	}

	//log.Debugf("(%p) AddFileIndex %s",  doc.textile, spew.Sdump(newFile))

	node, keys, err := doc.node.Textile.Node().AddNodeFromFiles([]*tpb.FileIndex{newFile})
	if err != nil {
		return nil, fmt.Errorf("AddNodeFromFiles error: %s", err.Error())
	}

	//log.Debugf("AddNodeFromFiles %s %s %s", spew.Sdump(node.Cid()), spew.Sdump(keys.Files), spew.Sdump(node.Links()))

	var caption = newVersion.Icon + documentIconAndNameSeperator + newVersion.Name

	if isMerge {
		caption = mergeFileCaption
	}

	block, err := doc.thread.AddFiles(node, "version", caption, keys.Files)
	if err != nil {
		return nil, fmt.Errorf("thread.AddFiles error: %s", err.Error())
	}
	//log.Debugf("(%p) Thread.AddFiles %s",  doc.textile, block.B58String())

	newVersion.Id = block.B58String()
	//fmt.Printf("saved new version %s... parent %s\n", newVersion.Id, newVersion.p)

	newVersion.User = doc.node.Textile.Node().Account().Address()
	newBlock, err := doc.node.Textile.Node().Block(block.B58String())
	if err != nil {
		log.Errorf("failed to get the block %s: %s", newBlock.Id, err.Error())
	}

	if newBlock != nil {
		newVersion.Date = newBlock.Date
	}

	var childrenIds []string
	var childrenMap = make(map[string]struct{})
	var traverseTree func(blocks []*pb.DocumentBlock)
	traverseTree = func(blocks []*pb.DocumentBlock) {
		var b DocumentBlockTypeDocument
		for _, block := range blocks {
			if (block.Type == pb.DocumentBlockType_NEW_PAGE || block.Type == pb.DocumentBlockType_PAGE) && block.Content != "" {
				err = json.Unmarshal([]byte(block.Content), &b)
				if err != nil {
					log.Errorf("DocumentBlockTypeDocument unmarshal error: %s", err.Error())
					continue
				}

				// do not add children to list more than once
				if _, exists := childrenMap[b.Id]; exists {
					continue
				}

				childrenIds = append(childrenIds, b.Id)
				childrenMap[b.Id] = struct{}{}
			}
			if len(block.Children) > 0 {
				traverseTree(block.Children)
			}
		}
	}
	traverseTree(newVersion.Blocks)

	/*err = doc.Modify(childrenIds, newVersion.Name, newVersion.Icon)
	if err != nil {
		log.Errorf("failed to modify doc's thread: %s", err.Error())
	}*/

	return newVersion, err
}

/*func (doc *Document) Modify(childrenIds []string, name string, icon string) error {
	var childrensStr string
	removedChild, addedChild := util.DiffStringSlice(doc.ChildrenIds, childrenIds)
	log.Debugf("doc modify: removed(%v) added(%v)", removedChild, addedChild)
	if len(removedChild) > 0 || len(addedChild) > 0 {
		if len(childrenIds) == 0 {
			childrensStr = "-"
		} else {
			childrensStr = strings.Join(childrenIds, ",")
		}
	}

	var newIconName string
	if name != doc.Name || icon != doc.Icon {
		newIconName = icon + documentIconAndNameSeperator + name
	}

	if newIconName == "" && childrensStr == "" {
		return nil
	}
	peers := doc.Peers()

	if len(addedChild) > 0 {
		isThreadChildren := make(map[string]struct{})
		for _, thrdId := range addedChild {
			isThreadChildren[thrdId] = struct{}{}
		}

		for _, childThread := range doc.node.Textile.Node().Threads() {
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
					peer := doc.node.Textile.Node().Datastore().Peers().Get(peerID)
					err := doc.node.Textile.Node().AddInvite(doc.node.Textile.Node().Threads(), peer.Address)
					if err != nil {
						log.Errorf("childThread.AddInvite(%s) error: %s", peerID, err.Error())
						return
					}
				}(childThread, tpeer.Id)

				childHasPeer[tpeer.Id] = struct{}{}
			}
		}
	}

/*	_, err := doc.Thread.(childrensStr, newIconName)
	for _, childrenId := range childrenIds {
		// it is ok to call on thread that not archived because this method has a check inside
		err = doc.textile.ArchiveThread(childrenId, false)
		if err != nil {
			log.Errorf("failed to unarchive: %s", err.Error())
		}
	}
	if err != nil {
		return err
	}


	return doc.textile.archiveIfAbandoned(removedChild...)

return nil
}*/

/*func  refreshChildren(documents []*pb.Document)  {
	children := d.LastVersion.ChildrenIds()
	for _, childDocId := range children {
		childDoc, err := d.node.Document(childDocId)
		if err != nil {
			log.Errorf("failed to convert child thread to doc: %s", err.Error())
			continue
		}
		childDoc.refreshChildren()
		d.Children = append(d.Children, childDoc)
	}
}*/

/*func (t *Document) childrenTree() []*Document {
	return t.childrenTreeIgnoreBranch("")
}*/

/*func (doc *Document) childrenTreeIgnoreBranch(ignoreThreadBranch string) []*Document {
	var threadById = make(map[string]*Thread)
	for _, th := range doc.node.Textile.Node().loadedThreads {
		threadById[th.Id] = th
	}

	threadById["root"] = doc.node.Textile.Node().AccountThread()

	var getChildren func([]string, []string, bool) []*Document
	getChildren = func(breadcrumbs []string, rootIds []string, isTemplate bool) []*Document {
		var tree []*Document
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

			docT, err := t.ToDocument()
			if err != nil {
				// ignore error because this is account or template thread
				continue
			}

			docT.Template = isTemplate
			if t.Key == "templates" {
				docT.Template = true
			}

			// do not traverse the tree when the thread's child is also some of the thread's parents
			if _, parentExistsInBreadcrumbs := breadcrumbsMap[id]; !parentExistsInBreadcrumbs && len(t.ChildrenIds) > 0 {
				docT.ChildrenIds = getChildren(append(breadcrumbs, id), t.ChildrenIds, docT.Template)
			}

			tree = append(tree, docT)
		}
		return tree
	}

	if doc == nil {
		return nil
	}

	tree := getChildren([]string{}, []string{doc.Id}, false)

	if len(tree) == 0 {
		return nil
	}

	return tree[0].ChildrenIds
}*/
