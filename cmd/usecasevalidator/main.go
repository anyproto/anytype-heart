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
	"gopkg.in/yaml.v3"

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
	objectInfo struct {
		Type, Name string
		SbType     smartblock.SmartBlockType
	}

	customInfo struct {
		isUsed         bool
		id, name       string
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
)

type Config struct {
	Validate    ValidationConfig `yaml:"validate"`
	Fix         FixConfig        `yaml:"fix"`
	Path        string           `yaml:"path"`
	Out         string           `yaml:"out"`
	ListObjects bool             `yaml:"list"`
}

type ValidationConfig struct {
	Enabled                bool `yaml:"enabled"`
	InsertCreator          bool `yaml:"insert_creator"`
	InsertAnalytics        bool `yaml:"insert_analytics"`
	RemoveAccountRelations bool `yaml:"remove_account_relations"`
}

type FixConfig struct {
	HomeObjectId                 string `yaml:"home_object_id"`
	SkipInvalidObjects           bool   `yaml:"skip_invalid_objects"`
	DeleteInvalidDetails         bool   `yaml:"delete_invalid_details"`
	DeleteInvalidDetailValues    bool   `yaml:"delete_invalid_detail_values"`
	DeleteInvalidRelationBlocks  bool   `yaml:"delete_invalid_relation_blocks"`
	DeleteInvalidCollectionItems bool   `yaml:"delete_invalid_collection_items"`
	SkipInvalidTypes             bool   `yaml:"skip_invalid_types"`
	RulesPath                    string `yaml:"rules_path"`
}

func (i customInfo) GetFormat() model.RelationFormat {
	return i.relationFormat
}

func (vc *ValidationConfig) isUpdateNeeded() bool {
	return vc.RemoveAccountRelations || vc.InsertAnalytics || vc.InsertCreator
}

func (fc *FixConfig) isUpdateNeeded() bool {
	return fc.DeleteInvalidDetails || fc.DeleteInvalidDetailValues || fc.DeleteInvalidRelationBlocks || fc.SkipInvalidTypes || fc.SkipInvalidObjects || fc.RulesPath != ""
}

func (c *Config) isUpdateNeeded() bool {
	return c.Fix.isUpdateNeeded() || c.Validate.isUpdateNeeded()
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

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func run() error {
	var configPath string
	configFlag := flag.NewFlagSet("config", flag.ExitOnError)
	configFlag.StringVar(&configPath, "config", "", "path to YAML config file")
	err := configFlag.Parse(os.Args[1:2])
	if err != nil {
		return err
	}

	config, err := loadConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if err = parseFlags(config); err != nil {
		return err
	}

	fileName := filepath.Base(config.Path)
	pathToNewZip := config.Out
	if pathToNewZip == "" {
		pathToNewZip = strings.TrimSuffix(config.Path, filepath.Ext(fileName)) + "_new.zip"
	}

	if config.Fix.RulesPath != "" {
		if err = readRules(config.Fix.RulesPath); err != nil {
			return err
		}
	}

	r, err := zip.OpenReader(config.Path)
	if err != nil {
		return fmt.Errorf("cannot open zip file %s: %w", config.Path, err)
	}
	defer r.Close()

	info, err := collectUseCaseInfo(r.File, fileName)
	if err != nil {
		return err
	}
	if info.profile == nil {
		fmt.Println("profile file does not present in archive")
	}

	updateNeeded := config.isUpdateNeeded()
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

	err = processFiles(info, writer, config)

	if config.ListObjects {
		listObjects(info)
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

func parseFlags(config *Config) error {
	flags := flag.NewFlagSet("flags", flag.ExitOnError)
	flags.StringVar(&config.Path, "path", config.Path, "Path to input zip archive")
	flags.StringVar(&config.Out, "out", config.Out, "Path to output zip archive")
	flags.BoolVar(&config.ListObjects, "list", config.ListObjects, "List all objects in archive")

	flags.BoolVar(&config.Validate.Enabled, "validate", config.Validate.Enabled, "Perform validation upon all objects")
	flags.BoolVar(&config.Validate.RemoveAccountRelations, "r", config.Validate.RemoveAccountRelations, "Remove account related relations")
	flags.BoolVar(&config.Validate.InsertAnalytics, "a", config.Validate.InsertAnalytics, "Insert analytics context and original id")
	flags.BoolVar(&config.Validate.InsertCreator, "creator", config.Validate.InsertCreator, "Set Anytype profile to LastModifiedDate and Creator")

	flags.StringVar(&config.Fix.HomeObjectId, "home_object", config.Fix.HomeObjectId, "Force home object id")
	flags.StringVar(&config.Fix.RulesPath, "rules", config.Fix.RulesPath, "Path to file with processing rules")

	err := flags.Parse(os.Args[2:])
	if err != nil {
		return fmt.Errorf("cannot parse flags: %w", err)
	}

	if config.Path == "" {
		return fmt.Errorf("path to zip archive should be specified")
	}
	return nil
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
		tk := strings.TrimPrefix(snapshot.Snapshot.Data.ObjectTypes[0], addr.ObjectTypeKeyToIdPrefix)

		info.objects[id] = objectInfo{
			Type:   tk,
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
				info.customTypesAndRelations[key] = customInfo{id: id, isUsed: false, relationFormat: model.RelationFormat(format), name: name}
			}
		case model.SmartBlockType_STType:
			uk := pbtypes.GetString(snapshot.Snapshot.Data.Details, bundle.RelationKeyUniqueKey.String())
			key := strings.TrimPrefix(uk, addr.ObjectTypeKeyToIdPrefix)
			info.types[id] = domain.TypeKey(key)
			if !bundle.HasObjectTypeByKey(domain.TypeKey(key)) {
				info.customTypesAndRelations[key] = customInfo{id: id, isUsed: false, name: name}
			}
		case model.SmartBlockType_SubObject:
			if strings.HasPrefix(id, addr.ObjectTypeKeyToIdPrefix) {
				key := strings.TrimPrefix(id, addr.ObjectTypeKeyToIdPrefix)
				info.types[id] = domain.TypeKey(key)
				if !bundle.HasObjectTypeByKey(domain.TypeKey(key)) {
					info.customTypesAndRelations[key] = customInfo{id: id, isUsed: false, name: name}
				}
			} else if strings.HasPrefix(id, addr.RelationKeyToIdPrefix) {
				key := strings.TrimPrefix(id, addr.RelationKeyToIdPrefix)
				info.relations[id] = domain.RelationKey(key)
				format := pbtypes.GetInt64(snapshot.Snapshot.Data.Details, bundle.RelationKeyRelationFormat.String())
				if !bundle.HasRelation(domain.RelationKey(key)) {
					info.customTypesAndRelations[key] = customInfo{id: id, isUsed: false, relationFormat: model.RelationFormat(format), name: name}
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

func processFiles(info *useCaseInfo, zw *zip.Writer, config *Config) error {
	var (
		incorrectFileFound bool
		writeNewFile       = config.isUpdateNeeded()
	)

	if info.profile != nil {
		data, err := processProfile(info, config.Fix.HomeObjectId)
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
		newData, err := processSnapshot(sn, info, config)
		if err != nil {
			if !(config.Fix.SkipInvalidObjects && errors.Is(err, errValidationFailed)) {
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

func processSnapshot(s *pb.SnapshotWithType, info *useCaseInfo, config *Config) ([]byte, error) {
	if config.Validate.InsertAnalytics {
		insertAnalyticsData(s.Snapshot, info)
	}

	if config.Validate.RemoveAccountRelations {
		removeAccountRelatedDetails(s.Snapshot)
	}

	if config.Validate.InsertCreator {
		insertCreatorInfo(s.Snapshot)
	}

	if config.Fix.RulesPath != "" {
		processRules(s.Snapshot)
	}

	if config.Validate.Enabled {
		if err := validate(s, info, config.Fix); err != nil {
			if errors.Is(err, errSkipObject) {
				// some validators register errors mentioning that object can be excluded
				return nil, nil
			}
			fmt.Println(err)
			return nil, errValidationFailed
		}
	}

	collectCustomObjectsUsageInfo(s, info)

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

func validate(snapshot *pb.SnapshotWithType, info *useCaseInfo, fixConfig FixConfig) (err error) {
	isValid := true
	id := pbtypes.GetString(snapshot.Snapshot.Data.Details, bundle.RelationKeyId.String())
	for _, v := range validators {
		if e := v(snapshot, info, fixConfig); e != nil {
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
			bundle.RelationKeyWorkspaceId.String(),
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

	if spaceDashboardId == "" {
		profile.SpaceDashboardId = "lastOpened"
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
		fmt.Printf("%s:\t%24s - %24s - %s\n", id[len(id)-4:], obj.SbType.String(), obj.Type, obj.Name)
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
	printCustomObjectsUsageInfo(info)
}

func isPlainFile(name string) bool {
	return strings.HasPrefix(name, export.Files) && !strings.HasPrefix(name, export.FilesObjects)
}
