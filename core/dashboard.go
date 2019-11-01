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

type Dashboard struct {
	thread *tcore.Thread `json:",inline"`
	node   *Anytype
}

func (dashboard *Dashboard) GetThread() *tcore.Thread {
	return dashboard.thread
}

func (dashboard *Dashboard) GetId() string {
	return dashboard.thread.Id
}

func (dashboard *Dashboard) GetType() BlockType {
	return BlockType_DASHBOARD
}

func (dashboard *Dashboard) GetVersion(id string) (SmartBlockVersion, error) {
	files, err := dashboard.node.Textile.Node().File(id)
	if err != nil {
		return nil, err
	}

	if len(files.Files) == 0 {
		return nil, errors.New("version block not found")
	}

	block := &Block{}

	plaintext, err := readFile(dashboard.node.Textile.Node(), files.Files[0].File)
	if err != nil {
		return nil, fmt.Errorf("readFile error: %s", err.Error())
	}

	err = proto.Unmarshal(plaintext, block)
	if err != nil {
		return nil, fmt.Errorf("page version proto unmarshal error: %s", err.Error())
	}

	version := &PageVersion{VersionId: files.Block, PageId: dashboard.GetId(), Date: files.Date, User: files.User.Address, Content: block.GetPage()}

	return version, nil
}

func (dashboard *Dashboard) GetLastVersion() (SmartBlockVersion, error) {
	versions, err := dashboard.GetVersions("", 1, false)
	if err != nil {
		return nil, err
	}

	if len(versions) == 0 {
		return nil, errorNotFound
	}

	return versions[0], nil
}

func (dashboard *Dashboard) GetVersions(offset string, limit int, metaOnly bool) ([]SmartBlockVersion, error) {
	files, err := dashboard.node.Textile.Node().Files(offset, limit, dashboard.thread.Id)
	if err != nil {
		return nil, err
	}

	var versions []SmartBlockVersion
	if len(files.Items) == 0 {
		return versions, nil
	}

	for _, item := range files.Items {
		version := &PageVersion{VersionId: item.Block, PageId: dashboard.GetId(), Date: item.Date, User: item.User.Address}

		if metaOnly {
			versions = append(versions, version)
			continue
		}

		block := &Block{}

		plaintext, err := readFile(dashboard.node.Textile.Node(), item.Files[0].File)
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

func (dashboard *Dashboard) AddVersion(newVersionInterface SmartBlockVersion) error {
	lastVersion, err := dashboard.GetLastVersion()

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

	if newVersion.VersionId != "" {
		/*	err = dashboard.Modify(dashboard.ChildrenIds, newVersion.Name, newVersion.Icon)
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

	newFile, err := dashboard.node.Textile.Node().AddFileIndex(mill, conf)
	if err != nil {
		return fmt.Errorf("AddFileIndex error: %s", err.Error())
	}

	//log.Debugf("(%p) AddFileIndex %s",  dashboard.textile, spew.Sdump(newFile))

	node, keys, err := dashboard.node.Textile.Node().AddNodeFromFiles([]*tpb.FileIndex{newFile})
	if err != nil {
		return fmt.Errorf("AddNodeFromFiles error: %s", err.Error())
	}

	//log.Debugf("AddNodeFromFiles %s %s %s", spew.Sdump(node.Cid()), spew.Sdump(keys.Files), spew.Sdump(node.Links()))

	var caption = newVersion.GetName()

	block, err := dashboard.thread.AddFiles(node, "version", caption, keys.Files)
	if err != nil {
		return fmt.Errorf("thread.AddFiles error: %s", err.Error())
	}
	//log.Debugf("(%p) Thread.AddFiles %s",  dashboard.textile, block.B58String())

	newVersion.VersionId = block.B58String()
	//fmt.Printf("saved new version %s... parent %s\n", newVersion.Id, newVersion.p)

	newVersion.User = dashboard.node.Textile.Node().Account().Address()
	newBlock, err := dashboard.node.Textile.Node().Block(block.B58String())
	if err != nil {
		log.Errorf("failed to get the block %s: %s", newBlock.Id, err.Error())
	}

	if newBlock != nil {
		newVersion.Date = newBlock.Date
	}

	return err
}

func (dashboard *Dashboard) GetExternalFields() *structpb.Struct {
	var name, icon string
	lastVersion, err := dashboard.GetLastVersion()
	if err == nil {
		switch lastVersion.(*DashboardVersion).Content.Style {
		case BlockContentDashboard_HOME:
			name, icon = "Home", ":housebuilding:"
		case BlockContentDashboard_ARCHIVE:
			name, icon = "Archive", ":wastebasket:"
		}
	}

	return &structpb.Struct{Fields: map[string]*structpb.Value{
		"name": {Kind: &structpb.Value_StringValue{name}},
		"icon": {Kind: &structpb.Value_StringValue{icon}},
	}}
}
