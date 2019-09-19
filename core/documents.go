package core

import (
	"crypto/rand"
	"fmt"
	"strings"

	"github.com/anytypeio/go-anytype-library/pb"

	libp2pc "github.com/libp2p/go-libp2p-crypto"
	"github.com/segmentio/ksuid"

	tcore "github.com/textileio/go-textile/core"
	tpb "github.com/textileio/go-textile/pb"
)

var errorDocumentCantFindParent = fmt.Errorf("can't find parent")

const documentIconAndNameSeperator = "\uF102"

func getIconAndNameFromPackedThreadName(joined string) (icon string, name string) {
	parts := strings.Split(joined, documentIconAndNameSeperator)

	if len(parts) == 0 {
		return "", ""
	}

	if len(parts) == 1 {
		return "", parts[0]
	}

	return parts[0], parts[1]
}

/*
// RemoveThread removes a thread
func (t *Textile) DuplicateDocument(id string, parentId string) (mh.Multihash, error) {
	var thrdToDuplicate, parentThrd *Thread
	if parentId == t.config.Account.Thread {
		parentThrd = t.ThreadByKey(t.config.Account.Address)
	}

	for _, th := range t.loadedThreads {
		if th.Id == id {
			thrdToDuplicate = th

		}

		if th.Id == parentId {
			parentThrd = th
		}

		if parentThrd != nil && thrdToDuplicate != nil {
			break
		}
	}

	if thrdToDuplicate == nil || parentThrd == nil {
		return nil, ErrThreadNotFound
	}

	threadsToDuplicate := childrenTreeFlattenMap(thrdToDuplicate.childrenTree(t.loadedThreads))
	threadsToDuplicate[thrdToDuplicate.Id] = thrdToDuplicate

	oldNewMap := map[string]*Thread{}
	oldThreadNewSK := map[string]libp2pc.PrivKey{}

	for _, thrd := range threadsToDuplicate {
		// make a new secret
		sk, _, err := libp2pc.GenerateEd25519Key(rand.Reader)
		if err != nil {
			return nil, err
		}
		oldThreadNewSK[thrd.Id] = sk
	}
	oldThreadMapMutex := sync.Mutex{}
	var errs = []string{}
	wg := sync.WaitGroup{}
	for _, thrd := range threadsToDuplicate {
		wg.Add(1)
		go func(thrd *Thread) {
			defer wg.Done()

			config := pb.AddThreadConfig{
				Name: thrd.Name,
				Key:  ksuid.New().String(),
				Schema: &pb.AddThreadConfig_Schema{
					Id: thrd.schemaId,
				},
			}

			// rename only the root threadю
			if thrd.Id == thrdToDuplicate.Id {
				config.Name = config.Name + " Copy"
			}

			sk := oldThreadNewSK[thrd.Id]

			for _, child := range thrd.ChildrenIds {
				var exists bool
				var childSk libp2pc.PrivKey

				if childSk, exists = oldThreadNewSK[child]; !exists {
					log.Errorf("failed to found old-new sk map for %s(children of %s(%s))", child, thrd.Name, thrd.Id)
					continue
				}

				id, err := peer.IDFromPrivateKey(childSk)
				if err != nil {
					log.Errorf("failed to get ID from PK: %s", err.Error())
					errs = append(errs, err.Error())
					continue
				}

				config.ChildrenIds = append(config.ChildrenIds, id.Pretty())
			}

			if thrd.ttype == pb.Thread_READ_ONLY && !thrd.writable(t.account.Address()) {
				config.Type = pb.Thread_OPEN
				config.Sharing = pb.Thread_SHARED
			} else {
				config.Type = thrd.ttype
				config.Sharing = thrd.sharing
			}

			if os.Getenv("ANYTYPE_DUPLICATE_READONLY") == "1" {
				config.Type = pb.Thread_READ_ONLY
			}

			newThrd, err := t.AddThread(config, sk, t.account.Address(), true, false)
			if err != nil {
				log.Errorf("failed to add the thread: %s", err.Error())
				errs = append(errs, err.Error())
				return
			}

			oldThreadMapMutex.Lock()
			oldNewMap[thrd.Id] = newThrd
			oldThreadMapMutex.Unlock()
		}(thrd)
	}
	wg.Wait()

	var titlePrefix = `{"blocks":[{"children":[],"content":"{\"text\":\"`
	var titleSuffix = `\"`
	for _, thrd := range threadsToDuplicate {
		files, err := t.Files("", 1, thrd.Id)
		if err != nil {
			log.Errorf("error while getting file for thread %s(%s)", thrd.Name, thrd.Id)
		}

		if len(files.Items) == 0 || len(files.Items[0].Files) == 0 {
			log.Errorf("DuplicateDocument: empty files for thread %s", thrd.Id)
			continue
		}

		plaintext, err := t.readFile(files.Items[0].Files[0].File)
		if err != nil {
			return nil, fmt.Errorf("readFile error: %s", err.Error())
		}

		uncompressedB, err := gzipUncompress(plaintext)

		if err == nil {
			plaintext = uncompressedB
		}

		for old, new := range oldNewMap {
			plaintext = bytes.Replace(plaintext, []byte(old), []byte(new.Id), -1)
		}

		newThread := oldNewMap[thrd.Id]

		_, oldName := getIconAndNameFromPackedThreadName(thrd.Name)
		_, newName := getIconAndNameFromPackedThreadName(newThread.Name)

		// todo: remove this shit
		plaintext = bytes.Replace(plaintext, []byte(titlePrefix+oldName+titleSuffix), []byte(titlePrefix+newName+titleSuffix), 1)

		newFile, err := t.writeJSON(plaintext)
		if err != nil {
			return nil, fmt.Errorf("writeJSON error: %s", err.Error())
		}

		node, keys, err := t.AddNodeFromFiles([]*pb.FileIndex{newFile})
		if err != nil {
			return nil, fmt.Errorf("AddNodeFromFiles error: %s", err.Error())
		}

		_, err = newThread.AddFiles(node, "", keys.Files)
		if err != nil {
			return nil, fmt.Errorf("thread.AddFiles error: %s", err.Error())
		}
	}

	newThreadId := oldNewMap[thrdToDuplicate.Id]
	parentThrd.ChildrenIds = append(parentThrd.ChildrenIds, newThreadId.Id)

	_, err := parentThrd.Modify(strings.Join(parentThrd.ChildrenIds, ","), "")

	if err != nil {
		return nil, err
	}

	return mh.FromB58String(newThreadId.Id)
}
*/
func (a *Anytype) DocumentView(id string) (*pb.Document, error) {
	t, err := a.ThreadView(id)
	if err != nil {
		return nil, err
	}

	if t.Schema != documentSchema {
		return nil, fmt.Errorf("not a document")
	}

	icon, name := getIconAndNameFromPackedThreadName(t.Name)

	doc := &pb.Document{
		Id:        t.Id,
		Name:      name,
		Initiator: t.Initiator,
		Type:      pb.Document_Type(t.Type),
		Icon:      icon,
		Archived:  false,
	}

	return doc, nil
}

func (a *Anytype) Document(id string) (*Document, error) {
	if doc, exists := a.documentsCache[id]; exists{
		return doc, nil
	}

	t := a.Thread(id)
	icon, name := getIconAndNameFromPackedThreadName(t.Name)
	tv, err := a.ThreadView(id)
	if err != nil {
		return nil, err
	}

	doc := &Document{
		thread: t,
		Document: &pb.Document{
			Id:        t.Id,
			Name:      name,
			Initiator: tv.Initiator,
			Type:      pb.Document_Type(tv.Type),
			Icon:      icon,
			Archived:  false,
			Children:  nil,
		},
		node: a,
	}


	return doc, nil
}

func (a *Anytype) Documents() ([]*Document, error) {
	var docs []*Document
	// todo: optimise memory
	for _, thrd := range a.Threads() {
		doc, err := a.Document(thrd.Id)
		if err != nil {
			continue
		}

		docs = append(docs, doc)
	}

	return docs, nil
}

func (a *Anytype) traverseDocumentsTree(childrenIds []string) ([]*pb.Document, error) {
	var children []*pb.Document

		for _, childId := range childrenIds{
			doc, err := a.Document(childId)
			if err != nil {
				// todo: maybe should skip instead of return?
				return nil, err
			}

			ver, err := doc.GetLastVersion()
			if err != nil {
				// todo: maybe should skip instead of return?
				return nil, err
			}

			if doc.Children == nil {
				childrenIds := ver.ChildrenIds()
				doc.Children, err = a.traverseDocumentsTree(childrenIds)
				if err != nil {
					// todo: maybe should skip instead of return?
					return nil, err
				}
			}

			children = append(children, doc.Document)
		}

		return children, nil
}

func (a *Anytype) RootDocuments() ([]*Document, error) {
	var hasParents = make(map[string]struct{})
	docs, err := a.Documents()
	if err != nil {
		return nil, err
	}

	for _, doc := range docs {
		children, err := doc.ChildrenIds()
		if err != nil {
			return nil, err
		}
		for _, child := range children {
			hasParents[child] = struct{}{}
		}
	}

	var rootDocs []*Document
	for _, doc := range docs {
		if _, exists := hasParents[doc.Id]; !exists {
			rootDocs = append(rootDocs, doc)
		}
	}

	return rootDocs, nil
}

func (a *Anytype) DocumentsTree() ([]*pb.Document, error) {
	rootDocs, err := a.RootDocuments()
	if err != nil {
		return nil, err
	}

	var rootDocsPb []*pb.Document
	for _, rootDoc := range rootDocs {
		childrenIds, err := rootDoc.ChildrenIds()
		if err != nil {
			return nil, err
		}

		rootDoc.Children, _ = a.traverseDocumentsTree(childrenIds)
		rootDocsPb = append(rootDocsPb, rootDoc.Document)
	}

	return rootDocsPb, nil
}

// AddDocument adds a document with a given name and secret key
func (a *Anytype) AddDocument(conf pb.AddDocumentConfig) (*Document, error) {
	if conf.Name == "" {
		conf.Name = defaultDocName
	}

	config := tpb.AddThreadConfig{
		Name: conf.Icon + documentIconAndNameSeperator + conf.Name,
		Key:  ksuid.New().String(),
		Schema: &tpb.AddThreadConfig_Schema{
			Id: documentSchema,
		},
		Sharing: tpb.Thread_Sharing(pbValForEnumString(tpb.Thread_Sharing_value, conf.Sharing)),
		Type:    tpb.Thread_Type(pbValForEnumString(tpb.Thread_Type_value, conf.Type)),
	}

	// make a new secret
	sk, _, err := libp2pc.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, err
	}

	var parentThread *tcore.Thread
	var thrd *tcore.Thread

	if conf.ParentToAddChild != "" {
		if conf.ParentToAddChild == "root" {
			parentThread = a.ThreadByKey(a.Config().Account.Address)
		} else {
			parentThread = a.Thread(conf.ParentToAddChild)
		}
		if parentThread == nil {
			return nil, errorDocumentCantFindParent
		}

		defer func() {
			if thrd == nil {
				// thread wasn't created so no need to update parent
				return
			}
			parentDoc, err  := a.Document(parentThread.Id)
			if err != nil {
				log.Errorf("failed to convert parent thread to doc: %s", err.Error())
				return
			}
			err = parentDoc.AddChild(thrd.Id, true)
			if err != nil {
				log.Errorf("Can't add document to the parent thread: %s", err.Error())
			}
		}()
	}

	thrd, err = a.AddThread(config, sk, a.Account().Address(), true, true)
	if err != nil {
		return nil, err
	}

	return a.Document(thrd.Id)
}

/*func documentsView(docs []*Document) []*pb.Document{
	var pbDocs []*pb.Document
	for _, doc := range docs {
		//pbDocs = append(pbDocs, doc.)
	}
}*/
