//go:build !nogrpcserver && !_test

package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/types"
	"github.com/hashicorp/go-multierror"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/export"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/constant"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type (
	relationWithFormat interface {
		GetFormat() model.RelationFormat
	}

	objectInfo struct {
		Type, Name string
		SbType     smartblock.SmartBlockType
	}

	customInfo struct {
		isUsed         bool
		id             string
		relationFormat model.RelationFormat
	}

	useCaseInfo struct {
		objects     map[string]objectInfo
		relations   map[string]domain.RelationKey
		types       map[string]domain.TypeKey
		templates   map[string]string
		options     map[string]domain.RelationKey
		fileObjects []string

		// big data
		files     map[string][]byte
		snapshots map[string]*pb.SnapshotWithType
		profile   []byte

		customTypesAndRelations map[string]customInfo

		useCase string
	}

	cliFlags struct {
		analytics, validate, creator   bool
		list, removeRelations, exclude bool
		collectCustomUsageInfo         bool
		path, rules, spaceDashboardId  string
	}
)

func (i customInfo) GetFormat() model.RelationFormat {
	return i.relationFormat
}

func (f cliFlags) isUpdateNeeded() bool {
	return f.analytics || f.creator || f.removeRelations || f.exclude || f.rules != ""
}

var (
	errIncorrectFileFound = fmt.Errorf("incorrect protobuf file was found")
	errValidationFailed   = fmt.Errorf("validation failed")
	errSkipObject         = fmt.Errorf("object is invalid, skip it")
)

func main() {
	if err := run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run() error {
	flags, err := getFlags()
	if err != nil {
		return err
	}
	fileName := filepath.Base(flags.path)
	pathToNewZip := strings.TrimSuffix(flags.path, filepath.Ext(fileName)) + "_new.zip"

	if flags.rules != "" {
		if err = readRules(flags.rules); err != nil {
			return err
		}
	}

	r, err := zip.OpenReader(flags.path)
	if err != nil {
		return fmt.Errorf("cannot open zip file %s: %w", flags.path, err)
	}
	defer r.Close()

	info, err := collectUseCaseInfo(r.File, fileName)
	if err != nil {
		return err
	}
	if info.profile == nil {
		fmt.Println("profile file does not present in archive")
	}

	updateNeeded := flags.isUpdateNeeded()
	var writer *zip.Writer

	if updateNeeded {
		zf, err := os.Create(pathToNewZip)
		if err != nil {
			return fmt.Errorf("failed to create output zip file: %w", err)
		}
		defer zf.Close()

		writer = zip.NewWriter(zf)
		defer writer.Close()
	}

	err = processFiles(info, writer, flags)

	if flags.list {
		listObjects(info)
	}

	if flags.collectCustomUsageInfo {
		printCustomObjectsUsageInfo(info)
	}

	if err != nil {
		if errors.Is(err, errIncorrectFileFound) {
			err = fmt.Errorf("provided zip contains some incorrect data. " +
				"Please examine errors above. You can change object in editor or add some rules to rules.json")
		} else {
			err = fmt.Errorf("an error occurred on protobuf files processing: %w", err)
		}
		_ = os.Remove(pathToNewZip)
		return err
	}

	if updateNeeded {
		fmt.Println("Processed zip is written to ", pathToNewZip)
	} else {
		fmt.Println("No changes to zip file were made")
	}

	return nil
}

func getFlags() (*cliFlags, error) {
	path := flag.String("path", "", "Path to zip archive")
	creator := flag.Bool("creator", false, "Set Anytype profile to LastModifiedDate and Creator")
	list := flag.Bool("list", false, "List all objects in archive")
	valid := flag.Bool("validate", false, "Perform validation upon all objects")
	removeRels := flag.Bool("r", false, "Remove account related relations")
	analytics := flag.Bool("a", false, "Insert analytics context and original id")
	rules := flag.String("rules", "", "Path to file with processing rules")
	exclude := flag.Bool("exclude", false, "Exclude objects that did not pass validation")
	custom := flag.Bool("c", false, "Collect usage information about custom types and relations")
	spaceDashboardId := flag.String("s", "", "Id of object to be set as Space Dashboard")

	flag.Parse()

	if *path == "" {
		return nil, fmt.Errorf("path to zip archive should be specified")
	}

	return &cliFlags{
		analytics:              *analytics,
		list:                   *list,
		removeRelations:        *removeRels,
		validate:               *valid,
		path:                   *path,
		creator:                *creator,
		rules:                  *rules,
		exclude:                *exclude,
		collectCustomUsageInfo: *custom,
		spaceDashboardId:       *spaceDashboardId,
	}, nil
}

func collectUseCaseInfo(files []*zip.File, fileName string) (info *useCaseInfo, err error) {
	info = &useCaseInfo{
		useCase:                 strings.TrimSuffix(fileName, filepath.Ext(fileName)),
		objects:                 make(map[string]objectInfo, len(files)-1),
		relations:               make(map[string]domain.RelationKey, len(files)-1),
		types:                   make(map[string]domain.TypeKey, len(files)-1),
		templates:               make(map[string]string),
		options:                 make(map[string]domain.RelationKey),
		files:                   make(map[string][]byte),
		snapshots:               make(map[string]*pb.SnapshotWithType, len(files)),
		fileObjects:             make([]string, 0),
		customTypesAndRelations: make(map[string]customInfo),
	}
	for _, f := range files {
		if f.FileInfo().IsDir() {
			continue
		}

		data, err := readData(f)
		if err != nil {
			return nil, err
		}

		if isPlainFile(f.Name) {
			info.files[f.Name] = data
			continue
		}

		if f.Name == constant.ProfileFile {
			info.profile = data
			continue
		}

		snapshot, err := extractSnapshotAndType(data, f.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to extract snapshot from file %s: %w", f.Name, err)
		}

		id := pbtypes.GetString(snapshot.Snapshot.Data.Details, bundle.RelationKeyId.String())
		name := pbtypes.GetString(snapshot.Snapshot.Data.Details, bundle.RelationKeyName.String())

		info.objects[id] = objectInfo{
			Type:   pbtypes.GetString(snapshot.Snapshot.Data.Details, bundle.RelationKeyType.String()),
			Name:   name,
			SbType: smartblock.SmartBlockType(snapshot.SbType),
		}

		info.snapshots[f.Name] = snapshot

		switch snapshot.SbType {
		case model.SmartBlockType_STRelation:
			uk := pbtypes.GetString(snapshot.Snapshot.Data.Details, bundle.RelationKeyUniqueKey.String())
			key := strings.TrimPrefix(uk, addr.RelationKeyToIdPrefix)
			info.relations[id] = domain.RelationKey(key)
			format := pbtypes.GetInt64(snapshot.Snapshot.Data.Details, bundle.RelationKeyRelationFormat.String())
			if !bundle.HasRelation(domain.RelationKey(key)) {
				info.customTypesAndRelations[key] = customInfo{id: id, isUsed: false, relationFormat: model.RelationFormat(format)}
			}
		case model.SmartBlockType_STType:
			uk := pbtypes.GetString(snapshot.Snapshot.Data.Details, bundle.RelationKeyUniqueKey.String())
			key := strings.TrimPrefix(uk, addr.ObjectTypeKeyToIdPrefix)
			info.types[id] = domain.TypeKey(key)
			if !bundle.HasObjectTypeByKey(domain.TypeKey(key)) {
				info.customTypesAndRelations[key] = customInfo{id: id, isUsed: false}
			}
		case model.SmartBlockType_SubObject:
			if strings.HasPrefix(id, addr.ObjectTypeKeyToIdPrefix) {
				key := strings.TrimPrefix(id, addr.ObjectTypeKeyToIdPrefix)
				info.types[id] = domain.TypeKey(key)
				if !bundle.HasObjectTypeByKey(domain.TypeKey(key)) {
					info.customTypesAndRelations[key] = customInfo{id: id, isUsed: false}
				}
			} else if strings.HasPrefix(id, addr.RelationKeyToIdPrefix) {
				key := strings.TrimPrefix(id, addr.RelationKeyToIdPrefix)
				info.relations[id] = domain.RelationKey(key)
				format := pbtypes.GetInt64(snapshot.Snapshot.Data.Details, bundle.RelationKeyRelationFormat.String())
				if !bundle.HasRelation(domain.RelationKey(key)) {
					info.customTypesAndRelations[key] = customInfo{id: id, isUsed: false, relationFormat: model.RelationFormat(format)}
				}
			}
		case model.SmartBlockType_Template:
			info.templates[id] = pbtypes.GetString(snapshot.Snapshot.Data.Details, bundle.RelationKeyTargetObjectType.String())
		case model.SmartBlockType_STRelationOption:
			info.options[id] = domain.RelationKey(pbtypes.GetString(snapshot.Snapshot.Data.Details, bundle.RelationKeyRelationKey.String()))
		case model.SmartBlockType_FileObject:
			info.fileObjects = append(info.fileObjects, id)
		}
	}
	return
}

func readData(f *zip.File) ([]byte, error) {
	rd, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("cannot open pb file %s: %w", f.Name, err)
	}
	defer rd.Close()
	data, err := io.ReadAll(rd)
	if err != nil {
		return nil, fmt.Errorf("cannot read data from file %s: %w", f.Name, err)
	}
	return data, nil
}

func processFiles(info *useCaseInfo, zw *zip.Writer, flags *cliFlags) error {
	var (
		incorrectFileFound bool
		writeNewFile       = flags.isUpdateNeeded()
	)

	if info.profile != nil {
		data, err := processProfile(info, flags.spaceDashboardId)
		if err != nil {
			return err
		}
		if writeNewFile {
			if err = saveDataToZip(zw, constant.ProfileFile, data); err != nil {
				return err
			}
		}
	}

	if writeNewFile {
		for name, data := range info.files {
			if err := saveDataToZip(zw, name, data); err != nil {
				return err
			}
		}
	}

	for name, sn := range info.snapshots {
		newData, err := processSnapshot(sn, info, flags)
		if err != nil {
			if !(flags.exclude && errors.Is(err, errValidationFailed)) {
				// just do not include object that failed validation
				incorrectFileFound = true
			}
			continue
		}

		if newData == nil || !writeNewFile {
			continue
		}
		if err = saveDataToZip(zw, name, newData); err != nil {
			return err
		}
	}

	if incorrectFileFound {
		return errIncorrectFileFound
	}
	return nil
}

func saveDataToZip(zw *zip.Writer, fileName string, data []byte) error {
	if strings.HasSuffix(fileName, ".pb.json") {
		// output of usecase validator is always an archive with protobufs
		fileName = strings.TrimSuffix(fileName, ".json")
	}
	nf, err := zw.Create(fileName)
	if err != nil {
		return fmt.Errorf("failed to create new file %s: %w", fileName, err)
	}
	if _, err = io.Copy(nf, bytes.NewReader(data)); err != nil {
		return fmt.Errorf("failed to copy snapshot to new file %s: %w", fileName, err)
	}
	return nil
}

func processSnapshot(s *pb.SnapshotWithType, info *useCaseInfo, flags *cliFlags) ([]byte, error) {
	if flags.analytics {
		insertAnalyticsData(s.Snapshot, info)
	}

	if flags.removeRelations {
		removeAccountRelatedDetails(s.Snapshot)
	}

	if flags.creator {
		insertCreatorInfo(s.Snapshot)
	}

	if flags.rules != "" {
		processRules(s.Snapshot)
	}

	if flags.validate {
		if err := validate(s, info); err != nil {
			if errors.Is(err, errSkipObject) {
				// some validators register errors mentioning that object can be excluded
				return nil, nil
			}
			fmt.Println(err)
			return nil, errValidationFailed
		}
	}

	if flags.collectCustomUsageInfo {
		collectCustomObjectsUsageInfo(s, info)
	}

	if s.SbType == model.SmartBlockType_AccountOld {
		return s.Snapshot.Marshal()
	}

	return s.Marshal()
}

func extractSnapshotAndType(data []byte, name string) (s *pb.SnapshotWithType, err error) {
	s = &pb.SnapshotWithType{}
	if strings.HasSuffix(name, ".json") {
		if err = jsonpb.UnmarshalString(string(data), s); err != nil {
			return nil, fmt.Errorf("cannot unmarshal snapshot from file %s: %w", name, err)
		}
		if s.SbType == model.SmartBlockType_AccountOld {
			cs := &pb.ChangeSnapshot{}
			if err = jsonpb.UnmarshalString(string(data), cs); err != nil {
				return nil, fmt.Errorf("cannot unmarshal snapshot from file %s: %w", name, err)
			}
			s = &pb.SnapshotWithType{
				Snapshot: cs,
				SbType:   model.SmartBlockType_AccountOld,
			}
		}
		return
	}

	if err = s.Unmarshal(data); err != nil {
		return nil, fmt.Errorf("cannot unmarshal snapshot from file %s: %w", name, err)
	}
	if s.SbType == model.SmartBlockType_AccountOld {
		cs := &pb.ChangeSnapshot{}
		if err = cs.Unmarshal(data); err != nil {
			return nil, fmt.Errorf("cannot unmarshal snapshot from file %s: %w", name, err)
		}
		s = &pb.SnapshotWithType{
			Snapshot: cs,
			SbType:   model.SmartBlockType_AccountOld,
		}
	}
	return s, nil
}

func validate(snapshot *pb.SnapshotWithType, info *useCaseInfo) (err error) {
	isValid := true
	id := pbtypes.GetString(snapshot.Snapshot.Data.Details, bundle.RelationKeyId.String())
	for _, v := range validators {
		if e := v(snapshot, info); e != nil {
			if errors.Is(e, errSkipObject) {
				return errSkipObject
			}
			isValid = false
			err = multierror.Append(err, e)
		}
	}
	if !isValid {
		return fmt.Errorf("object '%s' (name: '%s') is invalid: %w",
			id[len(id)-4:], pbtypes.GetString(snapshot.Snapshot.Data.Details, bundle.RelationKeyName.String()), err)
	}
	return nil
}

func insertAnalyticsData(s *pb.ChangeSnapshot, info *useCaseInfo) {
	if s == nil || s.Data == nil || len(s.Data.Blocks) == 0 {
		return
	}
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

func removeAccountRelatedDetails(s *pb.ChangeSnapshot) {
	for key := range s.Data.Details.Fields {
		switch key {
		case bundle.RelationKeyLastOpenedDate.String(),
			bundle.RelationKeyCreatedDate.String(),
			bundle.RelationKeySpaceId.String(),
			bundle.RelationKeyRelationFormatObjectTypes.String(),
			bundle.RelationKeySourceFilePath.String(),
			bundle.RelationKeyLinks.String(),
			bundle.RelationKeyBacklinks.String(),
			bundle.RelationKeyMentions.String(),
			bundle.RelationKeyIdentityProfileLink.String(),
			bundle.RelationKeyAddedDate.String(),
			bundle.RelationKeySyncDate.String(),
			bundle.RelationKeySyncError.String(),
			bundle.RelationKeySyncStatus.String(),
			bundle.RelationKeyChatId.String(),
			bundle.RelationKeyType.String():

			delete(s.Data.Details.Fields, key)
		}
	}
}

func insertCreatorInfo(s *pb.ChangeSnapshot) {
	s.Data.Details.Fields[bundle.RelationKeyCreator.String()] = pbtypes.String(addr.AnytypeProfileId)
	s.Data.Details.Fields[bundle.RelationKeyLastModifiedBy.String()] = pbtypes.String(addr.AnytypeProfileId)
}

func processProfile(info *useCaseInfo, spaceDashboardId string) ([]byte, error) {
	profile := &pb.Profile{}
	if err := profile.Unmarshal(info.profile); err != nil {
		e := fmt.Errorf("cannot unmarshal profile: %w", err)
		fmt.Println(e)
		return nil, e
	}
	profile.Name = ""
	profile.ProfileId = ""

	if spaceDashboardId != "" {
		profile.SpaceDashboardId = spaceDashboardId
		return profile.Marshal()
	}

	fmt.Println("spaceDashboardId = " + profile.SpaceDashboardId)
	if _, found := info.objects[profile.SpaceDashboardId]; !found && !slices.Contains([]string{"lastOpened"}, profile.SpaceDashboardId) {
		err := fmt.Errorf("failed to find Space Dashboard object '%s' among provided", profile.SpaceDashboardId)
		fmt.Println(err)
		return nil, err
	}
	return profile.Marshal()
}

func listObjects(info *useCaseInfo) {
	fmt.Println("\nUsecase '" + info.useCase + "' content:\n\n- General objects:")
	fmt.Println("Id:  " + strings.Repeat(" ", 12) + "Smartblock Type -" + strings.Repeat(" ", 17) + "Type Key - Name")
	for id, obj := range info.objects {
		if lo.Contains([]smartblock.SmartBlockType{
			smartblock.SmartBlockTypeObjectType,
			smartblock.SmartBlockTypeRelation,
			smartblock.SmartBlockTypeSubObject,
			smartblock.SmartBlockTypeTemplate,
			smartblock.SmartBlockTypeRelationOption,
		}, obj.SbType) {
			continue
		}
		key, found := info.types[obj.Type]
		if !found {
			fmt.Printf("type '%s' is not found in the archive\n", obj.Type)
		}
		fmt.Printf("%s:\t%24s - %24s - %s\n", id[len(id)-4:], obj.SbType.String(), key, obj.Name)
	}

	fmt.Println("\n- Types:")
	fmt.Println("Id:  " + strings.Repeat(" ", 24) + "Key - Name")
	for id, key := range info.types {
		obj := info.objects[id]
		fmt.Printf("%s:\t%24s - %s\n", id[len(id)-4:], key, obj.Name)
	}

	fmt.Println("\n- Relations:")
	fmt.Println("Id:  " + strings.Repeat(" ", 24) + "Key - Name")
	for id, key := range info.relations {
		obj := info.objects[id]
		fmt.Printf("%s:\t%24s - %s\n", id[len(id)-4:], key, obj.Name)
	}

	fmt.Println("\n- Templates:")
	fmt.Println("Id:  " + strings.Repeat(" ", 31) + "Name - Target object type id")
	for id, target := range info.templates {
		obj := info.objects[id]
		fmt.Printf("%s:\t%32s - %s\n", id[len(id)-4:], obj.Name, target)
	}

	fmt.Println("\n- Relation Options:")
	fmt.Println("Id:  " + strings.Repeat(" ", 31) + "Name - Relation key")
	for id, key := range info.options {
		obj := info.objects[id]
		fmt.Printf("%s:\t%32s - %s\n", id[len(id)-4:], obj.Name, key)
	}

	fmt.Println("\n- File Objects:")
	fmt.Println("Id:  " + strings.Repeat(" ", 31) + "Name")
	for _, id := range info.fileObjects {
		obj := info.objects[id]
		fmt.Printf("%s:\t%32s\n", id[len(id)-4:], obj.Name)
	}
}

func isPlainFile(name string) bool {
	return strings.HasPrefix(name, export.Files) && !strings.HasPrefix(name, export.FilesObjects)
}
