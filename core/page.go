package core

import (
	"errors"
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb"
	"github.com/gogo/protobuf/proto"
	structpb "github.com/golang/protobuf/ptypes/struct"
)

const (
	mergeFileCaption = "Merge"
	defaultDocName   = "Untitled"
	smartBlockSchema = "QmTShDQr2PeWEXE5D8r77Lz5NeyLK7NNRENXtQHTvqo9F5"
)

var errorNotFound = fmt.Errorf("not found")

type Page struct {
	SmartBlock
}

func (page *Page) GetVersion(id string) (BlockVersion, error) {
	files, err := page.node.Textile.Node().File(id)
	if err != nil {
		return nil, err
	}

	if len(files.Files) == 0 {
		return nil, errors.New("version block not found")
	}

	blockVersion := &pb.Block{}

	plaintext, err := readFile(page.node.Textile.Node(), files.Files[0].File)
	if err != nil {
		return nil, fmt.Errorf("readFile error: %s", err.Error())
	}

	err = proto.Unmarshal(plaintext, blockVersion)
	if err != nil {
		return nil, fmt.Errorf("page version proto unmarshal error: %s", err.Error())
	}

	version := &PageVersion{pb: blockVersion, VersionId: files.Block, Date: files.Date, User: files.User.Address}

	return version, nil
}

func (page *Page) GetCurrentVersion() (BlockVersion, error) {
	// todo: implement HEAD instead of always returning the last version
	versions, err := page.GetVersions("", 1, false)
	if err != nil {
		return nil, err
	}

	if len(versions) == 0 {
		return nil, errorNotFound
	}

	return versions[0], nil
}

func (page *Page) GetVersions(offset string, limit int, metaOnly bool) ([]BlockVersion, error) {
	files, err := page.node.Textile.Node().Files(offset, limit, page.thread.Id)
	if err != nil {
		return nil, err
	}

	var versions []BlockVersion
	if len(files.Items) == 0 {
		return versions, nil
	}

	for _, item := range files.Items {
		version := &PageVersion{VersionId: item.Block, Date: item.Date, User: item.User.Address}

		if metaOnly {
			versions = append(versions, version)
			continue
		}

		block := &pb.Block{}

		plaintext, err := readFile(page.node.Textile.Node(), item.Files[0].File)
		if err != nil {
			return nil, fmt.Errorf("readFile error: %s", err.Error())
		}

		err = proto.Unmarshal(plaintext, block)
		if err != nil {
			return nil, fmt.Errorf("page version proto unmarshal error: %s", err.Error())
		}

		version.pb = block
		versions = append(versions, version)
	}

	return versions, nil
}

func (page *Page) AddVersion(dependentBlocks map[string]BlockVersion, fields *structpb.Struct, children []string, content pb.IsBlockContent) error {
	newVersion := &PageVersion{pb: &pb.Block{}}

	if newVersionContent, ok := content.(*pb.BlockContentOfPage); !ok {
		return fmt.Errorf("unxpected smartblock type")
	} else {
		newVersion.pb.Content = newVersionContent
	}

	lastVersion, err := page.GetCurrentVersion()
	if lastVersion != nil {
		if fields == nil {
			fields = lastVersion.GetFields()
		}

		if content == nil {
			content = lastVersion.GetContent()
		}

		if dependentBlocks == nil {
			dependentBlocks = lastVersion.GetDependentBlocks()
		}

		if children == nil {
			children = lastVersion.GetChildrenIds()
		}

		lastVersionB, _ := proto.Marshal(lastVersion.(*PageVersion).pb.Content.(*pb.BlockContentOfPage).Page)
		newVersionB, _ := proto.Marshal(newVersion.pb.Content.(*pb.BlockContentOfPage).Page)
		if string(lastVersionB) == string(newVersionB) {
			log.Debugf("[MERGE] new version has the same blocks as the last version - ignore it")
			// do not insert the new version if no blocks have changed
			newVersion.VersionId = lastVersion.GetVersionId()
			newVersion.User = lastVersion.GetUser()
			newVersion.Date = lastVersion.GetDate()
		} else {
			fmt.Printf("version differs:new %s\n%s\n\n---\n\nlast %s\n%s", newVersion.VersionId, string(newVersionB), lastVersion.GetVersionId(), string(lastVersionB))
		}
	}

	if newVersion.VersionId != "" {
		/*	err = page.Modify(page.ChildrenIds, newVersion.Name, newVersion.Icon)
			if err != nil {
				return nil, err
			}*/
		return nil
	}

	newVersion.VersionId, newVersion.User, newVersion.Date, err = page.SmartBlock.AddVersion(newVersion.pb)
	if err != nil {
		return err
	}
	return nil
}
