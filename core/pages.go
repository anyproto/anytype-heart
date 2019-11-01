package core

/*
import (
	"fmt"
	"strings"
)


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

var errorPageCantFindParent = fmt.Errorf("can't find parent")

const pageIconAndNameSeperator = "\uF102"

func getIconAndNameFromPackedThreadName(joined string) (icon string, name string) {
	parts := strings.Split(joined, pageIconAndNameSeperator)

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
func (t *Textile) DuplicatePage(id string, parentId string) (mh.Multihash, error) {
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
			log.Errorf("DuplicatePage: empty files for thread %s", thrd.Id)
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

func (a *Anytype) PageView(id string) (*pb.Page, error) {
	t, err := a.Textile.Node().ThreadView(id)
	if err != nil {
		return nil, err
	}

	if t.Schema != smartBlockSchema {
		return nil, fmt.Errorf("not a page")
	}

	icon, name := getIconAndNameFromPackedThreadName(t.Name)

	page := &pb.Page{
		Id:        t.Id,
		Name:      name,
		Initiator: t.Initiator,
		Type:      pb.Page_Type(t.Type),
		Icon:      icon,
		Archived:  false,
	}

	return page, nil
}

func (a *Anytype) Page(id string) (*Page, error) {
	if page, exists := a.pagesCache[id]; exists {
		return page, nil
	}

	t := a.Textile.Node().Thread(id)
	icon, name := getIconAndNameFromPackedThreadName(t.Name)
	tv, err := a.Textile.Node().ThreadView(id)
	if err != nil {
		return nil, err
	}

	page := &Page{
		thread: t,
		Page: &pb.Page{
			Id:        t.Id,
			Name:      name,
			Initiator: tv.Initiator,
			Type:      pb.Page_Type(tv.Type),
			Icon:      icon,
			Archived:  false,
			Children:  nil,
		},
		node: a,
	}

	return page, nil
}

func (a *Anytype) Pages() ([]*Page, error) {
	var pages []*Page
	// todo: optimise memory
	for _, thrd := range a.Textile.Node().Threads() {
		page, err := a.Page(thrd.Id)
		if err != nil {
			continue
		}

		pages = append(pages, page)
	}

	return pages, nil
}

func (a *Anytype) traversePagesTree(childrenIds []string) ([]*pb.Page, error) {
	var children []*pb.Page

	for _, childId := range childrenIds {
		page, err := a.Page(childId)
		if err != nil {
			// todo: maybe should skip instead of return?
			return nil, err
		}

		ver, err := page.GetLastVersion()
		if err != nil {
			// todo: maybe should skip instead of return?
			return nil, err
		}

		if page.Children == nil {
			childrenIds := ver.ChildrenIds()
			page.Children, err = a.traversePagesTree(childrenIds)
			if err != nil {
				// todo: maybe should skip instead of return?
				return nil, err
			}
		}

		children = append(children, page.Page)
	}

	return children, nil
}

func (a *Anytype) RootPages() ([]*Page, error) {
	var hasParents = make(map[string]struct{})
	pages, err := a.Pages()
	if err != nil {
		return nil, err
	}

	for _, page := range pages {
		children, err := page.ChildrenIds()
		if err != nil {
			return nil, err
		}
		for _, child := range children {
			hasParents[child] = struct{}{}
		}
	}

	var rootDocs []*Page
	for _, page := range pages {
		if _, exists := hasParents[page.Id]; !exists {
			rootDocs = append(rootDocs, page)
		}
	}

	return rootDocs, nil
}

func (a *Anytype) PagesTree() ([]*pb.Page, error) {
	rootDocs, err := a.RootPages()
	if err != nil {
		return nil, err
	}

	var rootDocsPb []*pb.Page
	for _, rootDoc := range rootDocs {
		childrenIds, err := rootDoc.ChildrenIds()
		if err != nil {
			return nil, err
		}

		rootDoc.Children, _ = a.traversePagesTree(childrenIds)
		rootDocsPb = append(rootDocsPb, rootDoc.Page)
	}

	return rootDocsPb, nil
}

/*func pagesView(pages []*Page) []*pb.Page{
	var pbDocs []*pb.Page
	for _, page := range pages {
		//pbDocs = append(pbDocs, page.)
	}
}*/
