//go:build !nogrpcserver && !_test

package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/relation/relationutils"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/constant"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type relationWithFormat interface {
	GetFormat() model.RelationFormat
}

type useCaseInfo struct {
	ids      map[string]struct{}
	relsIds  map[string]struct{}
	typesIds map[string]struct{}

	useCase          string
	profileFileFound bool
}

const anytypeProfileFilename = addr.AnytypeProfileId + ".pb"

var (
	errIncorrectFileFound = fmt.Errorf("incorrect protobuf file was found")

	sbTypesToBeExcluded = map[model.SmartBlockType]struct{}{
		model.SmartBlockType_Workspace:   {},
		model.SmartBlockType_Widget:      {},
		model.SmartBlockType_ProfilePage: {},
	}
)

func main() {
	if err := run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("path to the archive as an argument expected")
	}
	path := os.Args[1]
	fileName := filepath.Base(path)
	pathToNewZip := strings.TrimSuffix(path, filepath.Ext(fileName)) + "_new.zip"

	r, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("cannot open zip file %s: %v", path, err)
	}
	defer r.Close()

	info := &useCaseInfo{
		useCase:          strings.TrimSuffix(fileName, filepath.Ext(fileName)),
		ids:              make(map[string]struct{}, len(r.File)-1),
		relsIds:          make(map[string]struct{}, len(r.File)-1),
		typesIds:         make(map[string]struct{}, len(r.File)-1),
		profileFileFound: false,
	}
	collectUseCaseInfo(r.File, info)
	if !info.profileFileFound {
		return fmt.Errorf("profile file does not present in archive")
	}

	zf, err := os.Create(pathToNewZip)
	if err != nil {
		return fmt.Errorf("failed to create output zip file: %v", err)
	}
	defer zf.Close()

	writer := zip.NewWriter(zf)
	defer writer.Close()

	if err := processFiles(r.File, writer, info); err != nil {
		if err == errIncorrectFileFound {
			fmt.Println("Provided zip contains some incorrect data. " +
				"Please examine errors above. You can change object in editor or add some rules to rules.json")
			_ = os.Remove(pathToNewZip)
		} else {
			fmt.Println("An error occurred on protobuf files processing:", err)
		}
		_ = os.Remove(pathToNewZip)
		return nil
	}
	fmt.Println("Processed zip is written to ", pathToNewZip)
	return nil
}

func collectUseCaseInfo(files []*zip.File, info *useCaseInfo) {
	for _, f := range files {
		if f.Name == constant.ProfileFile {
			info.profileFileFound = true
			continue
		}
		id := strings.TrimSuffix(f.Name, filepath.Ext(f.Name))
		info.ids[id] = struct{}{}
		if strings.HasPrefix(id, addr.RelationKeyToIdPrefix) {
			info.relsIds[strings.TrimPrefix(id, addr.RelationKeyToIdPrefix)] = struct{}{}
		}
		if strings.HasPrefix(id, addr.ObjectTypeKeyToIdPrefix) {
			info.typesIds[strings.TrimPrefix(id, addr.ObjectTypeKeyToIdPrefix)] = struct{}{}
		}
	}
}

func processFiles(files []*zip.File, zw *zip.Writer, info *useCaseInfo) error {
	var incorrectFileFound bool
	for _, f := range files {
		rd, err := f.Open()
		if err != nil {
			return fmt.Errorf("cannot open pb file %s: %v", f.Name, err)
		}
		if f.Name == anytypeProfileFilename {
			fmt.Println(anytypeProfileFilename, "is excluded")
			rd.Close()
			continue
		}
		data, err := processFile(rd, f.Name, info)
		if err != nil {
			incorrectFileFound = true
			continue
		}
		if data == nil {
			continue
		}
		nf, err := zw.Create(f.Name)
		if err != nil {
			return fmt.Errorf("failed to create new file %s: %v", f.Name, err)
		}
		if _, err = io.Copy(nf, bytes.NewReader(data)); err != nil {
			return fmt.Errorf("failed to copy snapshot to new file %s: %v", f.Name, err)
		}
	}
	if incorrectFileFound {
		return errIncorrectFileFound
	}
	return nil
}

func processFile(r io.ReadCloser, name string, info *useCaseInfo) ([]byte, error) {
	defer r.Close()

	id := strings.TrimSuffix(name, filepath.Ext(name))
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("cannot read data from file %s: %v", name, err)
	}

	if name == constant.ProfileFile {
		return processProfile(data, info)
	}

	snapshot, sbType, isOldAccount, err := extractSnapshotAndType(data, name)
	if err != nil {
		return nil, err
	}

	if _, found := sbTypesToBeExcluded[sbType]; found {
		fmt.Printf("Smartblock '%s' is excluded as has type %s\n", id, sbType.String())
		return nil, nil
	}
	fmt.Println(id, "\t", snapshot.Data.Details.Fields[bundle.RelationKeyName.String()].GetStringValue())

	isArchived := pbtypes.GetBool(snapshot.Data.Details, bundle.RelationKeyIsArchived.String())
	if isArchived {
		return nil, fmt.Errorf("object %s has isArchived == true", id)
	}

	if err = processAndValidate(snapshot, info); err != nil {
		return nil, err
	}

	if isOldAccount {
		return snapshot.Marshal()
	}

	typedSnapshot := &pb.SnapshotWithType{
		Snapshot: snapshot,
		SbType:   sbType,
	}

	return typedSnapshot.Marshal()
}

func extractSnapshotAndType(data []byte, name string) (s *pb.ChangeSnapshot, sbt model.SmartBlockType, isOldAccount bool, err error) {
	snapshotWithType := &pb.SnapshotWithType{}
	sbt = model.SmartBlockType_Page
	if err = snapshotWithType.Unmarshal(data); err != nil {
		return nil, sbt, false, fmt.Errorf("cannot unmarshal snapshot from file %s: %v", name, err)
	}
	if snapshotWithType.SbType == model.SmartBlockType_AccountOld {
		s = &pb.ChangeSnapshot{}
		isOldAccount = true
		if err = s.Unmarshal(data); err != nil {
			return nil, sbt, false, fmt.Errorf("cannot unmarshal snapshot from file %s: %v", name, err)
		}
	} else {
		s = snapshotWithType.Snapshot
		sbt = snapshotWithType.SbType
	}
	return s, sbt, isOldAccount, nil
}

func processAndValidate(snapshot *pb.ChangeSnapshot, info *useCaseInfo) error {
	id := pbtypes.GetString(snapshot.Data.Details, bundle.RelationKeyId.String())

	processRootBlock(snapshot, info)
	processExtraRelations(snapshot)
	processAccountRelatedDetails(snapshot)
	processRules(snapshot)

	if !strings.HasPrefix(id, addr.RelationKeyToIdPrefix) && !strings.HasPrefix(id, addr.ObjectTypeKeyToIdPrefix) {
		isValid := true
		for _, v := range validators {
			if err := v(snapshot, info); err != nil {
				isValid = false
			}
		}
		if !isValid {
			return fmt.Errorf("object '%s' is invalid", id)
		}
	}
	return nil
}

func processRootBlock(s *pb.ChangeSnapshot, info *useCaseInfo) {
	root := s.Data.Blocks[0]
	id := pbtypes.GetString(s.Data.Details, bundle.RelationKeyId.String())
	f := root.GetFields().GetFields()

	if f == nil {
		f = make(map[string]*types.Value)
	}
	root.Fields = &types.Struct{Fields: f}
	f["analyticsContext"] = pbtypes.String(info.useCase)
	if f["analyticsOriginalId"] == nil {
		f["analyticsOriginalId"] = pbtypes.String(id)
	}
}

func processExtraRelations(s *pb.ChangeSnapshot) {
	rels := relationutils.MigrateRelationModels(s.Data.ExtraRelations)
	relLinks := pbtypes.RelationLinks(s.Data.GetRelationLinks())
	for _, l := range rels {
		if !relLinks.Has(l.Key) {
			relLinks = append(relLinks, l)
		}
	}
	s.Data.RelationLinks = relLinks
}

func processAccountRelatedDetails(s *pb.ChangeSnapshot) {
	for key, _ := range s.Data.Details.Fields {
		switch key {
		case bundle.RelationKeyLastOpenedDate.String(), bundle.RelationKeyWorkspaceId.String(), bundle.RelationKeyCreatedDate.String():
			delete(s.Data.Details.Fields, key)
		case bundle.RelationKeyCreator.String(), bundle.RelationKeyLastModifiedBy.String():
			s.Data.Details.Fields[key] = pbtypes.String(addr.AnytypeProfileId)
		}
	}
}

func processProfile(data []byte, info *useCaseInfo) ([]byte, error) {
	profile := &pb.Profile{}
	if err := profile.Unmarshal(data); err != nil {
		e := fmt.Errorf("cannot unmarshal profile: %v", err)
		fmt.Println(e)
		return nil, e
	}
	profile.Name = ""
	profile.ProfileId = ""
	if _, found := info.ids[profile.SpaceDashboardId]; !found {
		err := fmt.Errorf("failed to find Space Dashboard object '%s' among provided", profile.SpaceDashboardId)
		fmt.Println(err)
		return nil, err
	}
	return profile.Marshal()
}
