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
	"golang.org/x/net/context"
	"gopkg.in/yaml.v3"

	"github.com/anyproto/anytype-heart/core/block/export"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
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

	fileInfo struct {
		isUsed bool
		isOld  bool
		source string
	}

	namedBytes struct {
		data []byte
		name string
	}

	useCaseInfo struct {
		objects      map[string]objectInfo
		relations    map[string]domain.RelationKey
		relIdsByKey  map[domain.RelationKey]string
		types        map[string]domain.TypeKey
		typeIdsByKey map[domain.TypeKey]string
		templates    map[string]string
		options      map[string]domain.RelationKey
		fileObjects  map[string]fileInfo

		// big data
		files     map[string][]byte
		snapshots map[string]*pb.SnapshotWithType
		profile   []byte

		customTypesAndRelations map[string]customInfo

		useCase string
	}
)

func (i *useCaseInfo) DeriveObjectID(_ context.Context, key domain.UniqueKey) (string, error) {
	id, found := i.relIdsByKey[domain.RelationKey(key.InternalKey())]
	if !found {
		return "", errors.New("relation not found")
	}
	return id, nil
}

type Config struct {
	Validate ValidationConfig `yaml:"validate"`
	Fix      FixConfig        `yaml:"fix"`
	Report   ReportConfig     `yaml:"report"`
	Path     string           `yaml:"path"`
	Out      string           `yaml:"out"`
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
	ApplyPrimitives              bool   `yaml:"apply_primitives"`
	RemoveRelationLinks          bool   `yaml:"remove_relation_links"`
	SkipUnusedFiles              bool   `yaml:"skip_unused_files"`
}

type ReportConfig struct {
	ListObjects bool `yaml:"list"`
	Changes     bool `yaml:"changes"`
	CustomUsage bool `yaml:"custom_usage"`
	FileUsage   bool `yaml:"file_usage"`
}

func (i customInfo) GetFormat() model.RelationFormat {
	return i.relationFormat
}

func (vc *ValidationConfig) isUpdateNeeded() bool {
	return vc.RemoveAccountRelations || vc.InsertAnalytics || vc.InsertCreator
}

func (fc *FixConfig) isUpdateNeeded() bool {
	return fc.DeleteInvalidDetails || fc.DeleteInvalidDetailValues || fc.DeleteInvalidRelationBlocks || fc.SkipInvalidTypes || fc.SkipInvalidObjects || fc.RulesPath != "" || fc.ApplyPrimitives || fc.RemoveRelationLinks || fc.SkipUnusedFiles
}

func (c *Config) isUpdateNeeded() bool {
	return c.Fix.isUpdateNeeded() || c.Validate.isUpdateNeeded()
}

func (c *Config) outFileName() string {
	if c.Out != "" {
		return c.Out
	}
	return strings.TrimSuffix(c.Path, filepath.Ext(c.Path)) + "_new.zip"
}

func (c *Config) fileName() string {
	return filepath.Base(c.Path)
}

var (
	errIncorrectFileFound = fmt.Errorf("incorrect protobuf file was found")
	errValidationFailed   = fmt.Errorf("validation failed")
)

func main() {
	if err := run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func loadConfig() (*Config, error) {
	var configPath string
	configFlag := flag.NewFlagSet("config", flag.ExitOnError)
	configFlag.StringVar(&configPath, "config", "", "path to YAML config file")
	err := configFlag.Parse(os.Args[1:2])
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	err = parseFlags(cfg)
	return cfg, err
}

func run() error {
	config, err := loadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
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

	fmt.Println("\nStarting validation of archive ", config.fileName())

	info, err := collectUseCaseInfo(r.File, config.fileName())
	if err != nil {
		return err
	}

	var writer *zip.Writer
	updateNeeded := config.isUpdateNeeded()
	outFileName := config.outFileName()

	if updateNeeded {
		zf, err := os.Create(outFileName)
		if err != nil {
			return fmt.Errorf("failed to create output zip file: %w", err)
		}
		defer zf.Close()

		writer = zip.NewWriter(zf)
		defer writer.Close()
	}

	reporter := &reporter{changes: make(map[string][]string)}
	err = processUsecase(info, writer, config, reporter)

	if err != nil {
		if errors.Is(err, errIncorrectFileFound) {
			err = fmt.Errorf("provided zip contains some incorrect data. " +
				"Please examine errors above. You can change object in editor or add some rules to rules.json")
		} else {
			err = fmt.Errorf("an error occurred on protobuf files processing: %w", err)
		}
		_ = os.Remove(outFileName)
		return err
	}

	reporter.report(config.Report, info)

	if updateNeeded {
		fmt.Println("Processed zip is written to ", outFileName)
	} else {
		fmt.Println("No changes to zip file were made")
	}

	return nil
}

func parseFlags(config *Config) error {
	flags := flag.NewFlagSet("flags", flag.ExitOnError)
	flags.StringVar(&config.Path, "path", config.Path, "Path to input zip archive")
	flags.StringVar(&config.Out, "out", config.Out, "Path to output zip archive")

	flags.BoolVar(&config.Validate.Enabled, "validate", config.Validate.Enabled, "Perform validation upon all objects")
	flags.BoolVar(&config.Validate.RemoveAccountRelations, "r", config.Validate.RemoveAccountRelations, "Remove account related relations")
	flags.BoolVar(&config.Validate.InsertAnalytics, "a", config.Validate.InsertAnalytics, "Insert analytics context and original id")
	flags.BoolVar(&config.Validate.InsertCreator, "creator", config.Validate.InsertCreator, "Set Anytype profile to LastModifiedDate and Creator")

	flags.StringVar(&config.Fix.HomeObjectId, "home_object", config.Fix.HomeObjectId, "Force home object id")
	flags.StringVar(&config.Fix.RulesPath, "rules", config.Fix.RulesPath, "Path to file with processing rules")

	flags.BoolVar(&config.Report.ListObjects, "list", config.Report.ListObjects, "List all objects in archive")
	flags.BoolVar(&config.Report.Changes, "report_changes", config.Report.Changes, "Print report on changes applied to the archive")
	flags.BoolVar(&config.Report.CustomUsage, "custom_usage", config.Report.CustomUsage, "Print report on usage of custom types and relations")
	flags.BoolVar(&config.Report.FileUsage, "file_usage", config.Report.FileUsage, "Print report on usage of files included in archive")

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
		relIdsByKey:             make(map[domain.RelationKey]string, len(files)-1),
		types:                   make(map[string]domain.TypeKey, len(files)-1),
		typeIdsByKey:            make(map[domain.TypeKey]string, len(files)-1),
		templates:               make(map[string]string),
		options:                 make(map[string]domain.RelationKey),
		files:                   make(map[string][]byte),
		snapshots:               make(map[string]*pb.SnapshotWithType, len(files)),
		fileObjects:             make(map[string]fileInfo, len(files)),
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
		var tk string
		if len(snapshot.Snapshot.Data.ObjectTypes) > 0 {
			tk = strings.TrimPrefix(snapshot.Snapshot.Data.ObjectTypes[0], addr.ObjectTypeKeyToIdPrefix)
		}

		info.objects[id] = objectInfo{
			Type:   tk,
			Name:   name,
			SbType: smartblock.SmartBlockType(snapshot.SbType),
		}

		prefix := export.ObjectsDirectory
		switch snapshot.SbType {
		case model.SmartBlockType_STRelation:
			uk := pbtypes.GetString(snapshot.Snapshot.Data.Details, bundle.RelationKeyUniqueKey.String())
			key := strings.TrimPrefix(uk, addr.RelationKeyToIdPrefix)
			info.relations[id] = domain.RelationKey(key)
			info.relIdsByKey[domain.RelationKey(key)] = id
			format := pbtypes.GetInt64(snapshot.Snapshot.Data.Details, bundle.RelationKeyRelationFormat.String())
			prefix = export.RelationsDirectory
			if !bundle.HasRelation(domain.RelationKey(key)) {
				info.customTypesAndRelations[key] = customInfo{id: id, isUsed: false, relationFormat: model.RelationFormat(format), name: name}
			}
		case model.SmartBlockType_STType:
			uk := pbtypes.GetString(snapshot.Snapshot.Data.Details, bundle.RelationKeyUniqueKey.String())
			key := strings.TrimPrefix(uk, addr.ObjectTypeKeyToIdPrefix)
			info.types[id] = domain.TypeKey(key)
			info.typeIdsByKey[domain.TypeKey(key)] = id
			prefix = export.TypesDirectory
			if !bundle.HasObjectTypeByKey(domain.TypeKey(key)) {
				info.customTypesAndRelations[key] = customInfo{id: id, isUsed: false, name: name}
			}
		case model.SmartBlockType_SubObject:
			if strings.HasPrefix(id, addr.ObjectTypeKeyToIdPrefix) {
				key := strings.TrimPrefix(id, addr.ObjectTypeKeyToIdPrefix)
				info.types[id] = domain.TypeKey(key)
				info.typeIdsByKey[domain.TypeKey(key)] = id
				prefix = export.TypesDirectory
				if !bundle.HasObjectTypeByKey(domain.TypeKey(key)) {
					info.customTypesAndRelations[key] = customInfo{id: id, isUsed: false, name: name}
				}
			} else if strings.HasPrefix(id, addr.RelationKeyToIdPrefix) {
				key := strings.TrimPrefix(id, addr.RelationKeyToIdPrefix)
				info.relations[id] = domain.RelationKey(key)
				info.relIdsByKey[domain.RelationKey(key)] = id
				prefix = export.RelationsDirectory
				format := pbtypes.GetInt64(snapshot.Snapshot.Data.Details, bundle.RelationKeyRelationFormat.String())
				if !bundle.HasRelation(domain.RelationKey(key)) {
					info.customTypesAndRelations[key] = customInfo{id: id, isUsed: false, relationFormat: model.RelationFormat(format), name: name}
				}
			}
		case model.SmartBlockType_Template:
			info.templates[id] = pbtypes.GetString(snapshot.Snapshot.Data.Details, bundle.RelationKeyTargetObjectType.String())
			prefix = export.TemplatesDirectory
		case model.SmartBlockType_STRelationOption:
			info.options[id] = domain.RelationKey(pbtypes.GetString(snapshot.Snapshot.Data.Details, bundle.RelationKeyRelationKey.String()))
			prefix = export.RelationsOptionsDirectory
		case model.SmartBlockType_FileObject:
			info.fileObjects[id] = fileInfo{source: pbtypes.GetString(snapshot.Snapshot.Data.Details, bundle.RelationKeySource.String())}
			prefix = export.FilesObjects
		case model.SmartBlockType_File:
			info.fileObjects[id] = fileInfo{source: pbtypes.GetString(snapshot.Snapshot.Data.Details, bundle.RelationKeySource.String()), isOld: true}
			prefix = export.ObjectsDirectory
		}

		fName := f.Name
		if !strings.HasPrefix(fName, prefix) {
			fName = filepath.Join(prefix, fName)
		}
		info.snapshots[fName] = snapshot
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

func processUsecase(info *useCaseInfo, zw *zip.Writer, config *Config, reporter *reporter) error {
	var (
		incorrectFileFound bool
		saveUpdatedFile    = config.isUpdateNeeded()
	)

	if info.profile != nil {
		data, err := processProfile(info, config.Fix.HomeObjectId, reporter)
		if err != nil {
			return err
		}
		if saveUpdatedFile {
			if err = saveDataToZip(zw, constant.ProfileFile, data); err != nil {
				return err
			}
		}
	}

	updatedFileObjects := make(map[string]namedBytes, len(info.fileObjects))
	for name, sn := range info.snapshots {
		newData, err := processSnapshot(sn, info, config, reporter)
		if err != nil {
			if !(config.Fix.SkipInvalidObjects && errors.Is(err, errValidationFailed)) {
				// just do not include object that failed validation
				incorrectFileFound = true
			}
			continue
		}

		if newData == nil || !saveUpdatedFile {
			continue
		}

		if sn.SbType == model.SmartBlockType_FileObject || sn.SbType == model.SmartBlockType_File {
			// we have to save file objects in the end, because we need to check file usage
			updatedFileObjects[getId(sn)] = namedBytes{data: newData, name: name}
			continue
		}

		if err = saveDataToZip(zw, name, newData); err != nil {
			return err
		}
	}

	if saveUpdatedFile {
		if err := saveFiles(zw, info, config, updatedFileObjects, reporter); err != nil {
			return err
		}
	}

	if incorrectFileFound {
		return errIncorrectFileFound
	}
	return nil
}

func saveFiles(zw *zip.Writer, info *useCaseInfo, config *Config, fileObjects map[string]namedBytes, reporter *reporter) (err error) {
	sources := make(map[string]struct{})
	for id, fileObject := range fileObjects {
		fInfo, ok := info.fileObjects[id]
		if config.Fix.SkipUnusedFiles && !fInfo.isUsed {
			reporter.addSkipMsg(id, "unused file object")
			continue
		}
		if ok {
			sources[fInfo.source] = struct{}{}
		}
		if err = saveDataToZip(zw, fileObject.name, fileObject.data); err != nil {
			return err
		}
	}

	for name, data := range info.files {
		_, found := sources[name]
		if config.Fix.SkipUnusedFiles && !found {
			reporter.addSkipMsg(name, "unused file")
			continue
		}
		if err = saveDataToZip(zw, name, data); err != nil {
			return err
		}
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

func processSnapshot(s *pb.SnapshotWithType, info *useCaseInfo, config *Config, reporter *reporter) ([]byte, error) {
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

	if config.Fix.ApplyPrimitives {
		applyPrimitives(s, info, reporter)
	}

	if config.Fix.RemoveRelationLinks {
		removeRelationLinks(s, reporter)
	}

	if config.Validate.Enabled {
		skip, err := validate(s, info, config.Fix, reporter)
		if skip {
			// some validators register errors mentioning that object can be excluded
			return nil, nil
		}
		if err != nil {
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

func validate(snapshot *pb.SnapshotWithType, info *useCaseInfo, fixConfig FixConfig, reporter *reporter) (shouldSkip bool, err error) {
	for _, v := range validators {
		skip, e := v(snapshot, info, fixConfig, reporter)
		if skip {
			return true, nil
		}
		if e != nil {
			err = multierror.Append(err, e)
		}
	}
	if err != nil {
		id := pbtypes.GetString(snapshot.Snapshot.Data.Details, bundle.RelationKeyId.String())
		return false, fmt.Errorf("object '%s' (name: '%s') is invalid: %w",
			id[len(id)-4:], pbtypes.GetString(snapshot.Snapshot.Data.Details, bundle.RelationKeyName.String()), err)
	}
	return false, nil
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

func applyPrimitives(s *pb.SnapshotWithType, info *useCaseInfo, reporter *reporter) {
	id := getId(s)
	if s.SbType == model.SmartBlockType_Page {
		relationsToDelete := make([]string, 0, 3)
		for _, rel := range []string{bundle.RelationKeyLayout.String(), bundle.RelationKeyLayoutAlign.String()} {
			_, found := s.Snapshot.Data.Details.Fields[rel]
			if found {
				relationsToDelete = append(relationsToDelete, rel)
				delete(s.Snapshot.Data.Details.Fields, rel)
			}
		}

		featuredRelations := pbtypes.GetStringList(s.Snapshot.Data.Details, bundle.RelationKeyFeaturedRelations.String())
		if featuredRelations != nil {
			if slices.Contains(featuredRelations, bundle.RelationKeyDescription.String()) {
				reporter.addMsg(id, "primitives: leave only description in featured relations")
				s.Snapshot.Data.Details.Fields[bundle.RelationKeyFeaturedRelations.String()] = pbtypes.StringList([]string{bundle.RelationKeyDescription.String()})
			} else {
				relationsToDelete = append(relationsToDelete, bundle.RelationKeyFeaturedRelations.String())
				delete(s.Snapshot.Data.Details.Fields, bundle.RelationKeyFeaturedRelations.String())
			}
		}

		if len(relationsToDelete) > 0 {
			reporter.addMsg(id, fmt.Sprintf("primitives: layout related details deleted: [%s]", strings.Join(relationsToDelete, ",")))
		}
	}

	if s.SbType != model.SmartBlockType_STType {
		return
	}

	if _, found := s.Snapshot.Data.Details.Fields[bundle.RelationKeyRecommendedFeaturedRelations.String()]; found {
		return
	}

	details := domain.NewDetailsFromProto(s.Snapshot.Data.Details).
		CopyOnlyKeys(bundle.RelationKeyRecommendedRelations, bundle.RelationKeyRecommendedLayout)

	relationIds := details.GetStringList(bundle.RelationKeyRecommendedRelations)
	bundleIds := make([]string, 0, len(relationIds))
	for _, relationId := range relationIds {
		key, ok := info.relations[relationId]
		if !ok {
			continue
		}
		bundleIds = append(bundleIds, key.BundledURL())
	}
	details.SetStringList(bundle.RelationKeyRecommendedRelations, bundleIds)

	typeKey := domain.TypeKey(pbtypes.GetString(s.Snapshot.Data.Details, bundle.RelationKeyUniqueKey.String()))
	_, _, err := relationutils.FillRecommendedRelations(nil, info, details, typeKey)
	if err != nil {
		fmt.Println(err)
		return
	}

	reporter.addMsg(id, "primitives: recommended relations lists are refilled")
	delete(s.Snapshot.Data.Details.Fields, bundle.RelationKeyRecommendedRelations.String())
	s.Snapshot.Data.Details = pbtypes.StructMerge(s.Snapshot.Data.Details, details.ToProto(), true)
}

func removeRelationLinks(s *pb.SnapshotWithType, reporter *reporter) {
	s.Snapshot.Data.RelationLinks = nil
	reporter.addMsg(getId(s), "relation links removed")
}

func processProfile(info *useCaseInfo, spaceDashboardId string, reporter *reporter) ([]byte, error) {
	profile := &pb.Profile{}
	if err := profile.Unmarshal(info.profile); err != nil {
		err = fmt.Errorf("cannot unmarshal profile: %w", err)
		fmt.Println(err)
		return nil, err
	}
	profile.Name = ""
	profile.ProfileId = ""

	if spaceDashboardId != "" {
		profile.SpaceDashboardId = spaceDashboardId
		return profile.Marshal()
	}

	if profile.SpaceDashboardId == "" {
		profile.SpaceDashboardId = "lastOpened"
		return profile.Marshal()
	}

	if _, found := info.objects[profile.SpaceDashboardId]; !found && !slices.Contains([]string{"lastOpened", "graph"}, profile.SpaceDashboardId) {
		reporter.addMsg("profile", fmt.Sprintf("spaceDashboardId '%s' not found, so setting 'lastOpened' value", profile.SpaceDashboardId))
		profile.SpaceDashboardId = "lastOpened"
	}
	return profile.Marshal()
}

func isPlainFile(name string) bool {
	return strings.HasPrefix(name, export.Files) && !strings.HasPrefix(name, export.FilesObjects)
}
