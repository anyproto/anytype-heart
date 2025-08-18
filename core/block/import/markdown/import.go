package markdown

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/import/common/source"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/pkg/lib/schema"
	"github.com/anyproto/anytype-heart/pkg/lib/schema/yaml"
	"github.com/anyproto/anytype-heart/util/constant"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var (
	emojiAproxRegexp = regexp.MustCompile(`[\x{2194}-\x{329F}\x{1F000}-\x{1FADF}]`)
	log              = logging.Logger("markdown-import")
)

const numberOfStages = 9 // 8 cycles to get snapshots and 1 cycle to create objects

type Markdown struct {
	blockConverter *mdConverter
	service        *collection.Service
}

const (
	Name               = "Markdown"
	rootCollectionName = "Markdown Import"
	propIdPrefix       = "import_prop_"
	typeIdPrefix       = "import_type_"
)

func New(tempDirProvider core.TempDirProvider, service *collection.Service) common.Converter {
	bc := newMDConverter(tempDirProvider)
	return &Markdown{
		blockConverter: bc,
		service:        service,
	}
}

func (m *Markdown) Name() string {
	return Name
}

func (m *Markdown) GetParams(req *pb.RpcObjectImportRequest) *pb.RpcObjectImportRequestMarkdownParams {
	if p := req.GetMarkdownParams(); p != nil {
		return p
	} else {
		return &pb.RpcObjectImportRequestMarkdownParams{}
	}
}

func (m *Markdown) GetImage() ([]byte, int64, int64, error) {
	return nil, 0, 0, nil
}

func (m *Markdown) GetSnapshots(ctx context.Context, req *pb.RpcObjectImportRequest, progress process.Progress) (*common.Response, *common.ConvertError) {
	params := m.GetParams(req)
	if len(params.Path) == 0 {
		return nil, nil
	}
	allErrors := common.NewError(req.Mode)
	si := NewSchemaImporter()
	m.blockConverter.SetSchemaImporter(si)

	allSnapshots, allRootObjectsIds := m.processFiles(req, progress, params.Path, allErrors)
	if allErrors.ShouldAbortImport(len(params.Path), req.Type) {
		return nil, allErrors
	}
	var (
		rootObjectID string
		err          error
		widgetType   model.BlockContentWidgetLayout
	)
	if params.CreateDirectoryPages && len(allRootObjectsIds) == 1 {
		rootObjectID = allRootObjectsIds[0]
		widgetType = model.BlockContentWidget_Tree
	} else {
		if params.CreateDirectoryPages {
			log.Warnf("%d root pages found, creating collection", len(allRootObjectsIds))
		}
		allSnapshots, rootObjectID, err = m.createRootCollection(allSnapshots, allRootObjectsIds)
		if err != nil {
			allErrors.Add(err)
			if allErrors.ShouldAbortImport(len(params.Path), req.Type) {
				return nil, allErrors
			}
		}
	}

	var typesCreated []domain.TypeKey
	for _, snapshot := range allSnapshots {
		if snapshot.Snapshot.SbType == smartblock.SmartBlockTypeObjectType {
			uk := snapshot.Snapshot.Data.Details.GetString(bundle.RelationKeyUniqueKey)
			uniqueKey, err := domain.GetTypeKeyFromRawUniqueKey(uk)
			if err != nil {
				log.Warnf("type widgets, failed to get type key from unique key %s: %v", uk, err)
				continue
			}
			typesCreated = append(typesCreated, uniqueKey)
		}
	}
	if allErrors.IsEmpty() {
		return &common.Response{Snapshots: allSnapshots, RootObjectID: rootObjectID, RootObjectWidgetType: widgetType, TypesCreated: typesCreated}, nil
	}
	return &common.Response{Snapshots: allSnapshots, RootObjectID: rootObjectID, RootObjectWidgetType: widgetType, TypesCreated: typesCreated}, allErrors
}

func (m *Markdown) processFiles(req *pb.RpcObjectImportRequest, progress process.Progress, paths []string, allErrors *common.ConvertError) ([]*common.Snapshot, []string) {
	var (
		allSnapshots      []*common.Snapshot
		allRootObjectsIds []string
	)

	// Check if all paths share the same parent directory
	if len(paths) > 1 {
		if commonParent := findCommonParentDir(paths); commonParent != "" {
			// All paths are within the same parent directory
			// Import the parent directory with filtering for selected paths
			snapshots, rootObjectsIds := m.getSnapshotsAndRootObjectsIdsWithFilter(req, progress, commonParent, paths, allErrors)
			if !allErrors.ShouldAbortImport(len(paths), req.Type) {
				allSnapshots = append(allSnapshots, snapshots...)
				allRootObjectsIds = append(allRootObjectsIds, rootObjectsIds...)
			}
			return allSnapshots, allRootObjectsIds
		}
	}

	// Process paths individually (original behavior)
	for _, path := range paths {
		snapshots, rootObjectsIds := m.getSnapshotsAndRootObjectsIds(req, progress, path, allErrors)
		if allErrors.ShouldAbortImport(len(paths), req.Type) {
			return nil, nil
		}
		allSnapshots = append(allSnapshots, snapshots...)
		allRootObjectsIds = append(allRootObjectsIds, rootObjectsIds...)
	}
	return allSnapshots, allRootObjectsIds
}

// findCommonParentDir checks if all paths share the same parent directory
func findCommonParentDir(paths []string) string {
	if len(paths) < 2 {
		return ""
	}

	// Get absolute paths and their parents
	var parents []string
	for _, p := range paths {
		absPath, err := filepath.Abs(p)
		if err != nil {
			return "" // If we can't get absolute path, bail out
		}
		parents = append(parents, filepath.Dir(absPath))
	}

	// Check if all parents are the same
	firstParent := parents[0]
	for _, parent := range parents[1:] {
		if parent != firstParent {
			return ""
		}
	}

	return firstParent
}

func (m *Markdown) createRootCollection(allSnapshots []*common.Snapshot, allRootObjectsIds []string) ([]*common.Snapshot, string, error) {
	rootCollection := common.NewImportCollection(m.service)
	settings := common.NewImportCollectionSetting(
		common.WithCollectionName(rootCollectionName),
		common.WithTargetObjects(allRootObjectsIds),
		common.WithAddDate(),
		common.WithRelations(),
	)
	rootCol, err := rootCollection.MakeImportCollection(settings)
	if err != nil {
		return nil, "", err
	}

	var rootCollectionID string
	if rootCol != nil {
		allSnapshots = append(allSnapshots, rootCol)
		rootCollectionID = rootCol.Id
	}
	return allSnapshots, rootCollectionID, nil
}

func wrapCallbackEnabler(enable bool, f func(files *fileContainer, progress process.Progress, details map[string]*domain.Details, allErrors *common.ConvertError)) func(*fileContainer, process.Progress, map[string]*domain.Details, *common.ConvertError) {
	if !enable {
		return func(_ *fileContainer, _ process.Progress, _ map[string]*domain.Details, _ *common.ConvertError) {
		}
	}
	return f
}
func (m *Markdown) getSnapshotsAndRootObjectsIds(
	req *pb.RpcObjectImportRequest,
	progress process.Progress,
	path string,
	allErrors *common.ConvertError,
) ([]*common.Snapshot, []string) {
	return m.getSnapshotsAndRootObjectsIdsWithFilter(req, progress, path, nil, allErrors)
}

func (m *Markdown) getSnapshotsAndRootObjectsIdsWithFilter(
	req *pb.RpcObjectImportRequest,
	progress process.Progress,
	path string,
	selectedPaths []string,
	allErrors *common.ConvertError,
) ([]*common.Snapshot, []string) {
	importSource := source.GetSource(path)
	if importSource == nil {
		return nil, nil
	}
	defer importSource.Close()

	// Initialize source with filtering if selectedPaths are provided
	var err error
	if filterSource, ok := importSource.(source.FilterableSource); ok && len(selectedPaths) > 0 {
		err = filterSource.InitializeWithFilter(path, selectedPaths)
	} else {
		err = importSource.Initialize(path)
	}

	if err != nil {
		allErrors.Add(err)
		if allErrors.ShouldAbortImport(0, model.Import_Markdown) {
			return nil, nil
		}
	}
	// Load schemas if available
	if err := m.blockConverter.schemaImporter.LoadSchemas(importSource, allErrors); err != nil {
		log.Warnf("failed to load schemas: %v", err)
	}

	params := m.GetParams(req)
	if m.blockConverter.schemaImporter.HasSchemas() {
		// we import from anytype markdown files. disable tree structure and properties as blocks
		params.CreateDirectoryPages = false
		params.IncludePropertiesAsBlock = false
	}

	files := m.blockConverter.markdownToBlocks(path, importSource, allErrors, params.CreateDirectoryPages)
	pathsCount := len(req.GetMarkdownParams().Path)
	if allErrors.ShouldAbortImport(pathsCount, req.Type) {
		return nil, nil
	}

	progress.SetTotal(int64(numberOfStages * len(files.byPath)))
	details := make(map[string]*domain.Details)

	if m.processImportStep(pathsCount, files, progress, allErrors, details, m.setInboundLinks) ||
		m.processImportStep(pathsCount, files, progress, allErrors, details, m.setNewID) ||
		m.processImportStep(pathsCount, files, progress, allErrors, details, m.processObjectProperties) ||
		m.processImportStep(pathsCount, files, progress, allErrors, details, m.addLinkToObjectBlocks) ||
		m.processImportStep(pathsCount, files, progress, allErrors, details, m.linkPagesWithRootFile) ||
		// todo: understand why we need this
		m.processImportStep(pathsCount, files, progress, allErrors, details, m.addLinkBlocks) ||
		m.processImportStep(pathsCount, files, progress, allErrors, details, m.fillEmptyBlocks) ||
		m.processImportStep(pathsCount, files, progress, allErrors, details, wrapCallbackEnabler(params.IncludePropertiesAsBlock, m.addPropertyBlocks)) ||
		m.processImportStep(pathsCount, files, progress, allErrors, details, m.addChildBlocks) {
		return nil, nil
	}

	var rootObjectsIds []string
	if params.CreateDirectoryPages {
		for _, file := range files.byPath {
			if file.IsRootDirPage {
				rootObjectsIds = append(rootObjectsIds, file.PageID)
				break
			}
		}
	} else {
		rootObjectsIds = m.retrieveRootObjectsIds(files)
	}
	return m.createSnapshots(req, pathsCount, files, progress, details, allErrors), rootObjectsIds
}

func (m *Markdown) processImportStep(pathCount int,
	files *fileContainer,
	progress process.Progress,
	allErrors *common.ConvertError,
	details map[string]*domain.Details,
	callback func(*fileContainer, process.Progress, map[string]*domain.Details, *common.ConvertError),
) (abortImport bool) {
	callback(files, progress, details, allErrors)
	return allErrors.ShouldAbortImport(pathCount, model.Import_Markdown)
}

func (m *Markdown) retrieveCollectionObjectsIds(csvFileName string, files *fileContainer) []string {
	ext := filepath.Ext(csvFileName)
	csvDir := strings.TrimSuffix(csvFileName, ext)
	var collectionsObjectsIds []string
	for name, file := range files.byPath {
		fileExt := filepath.Ext(name)
		if filepath.Dir(name) == csvDir && strings.EqualFold(fileExt, ".md") {
			file.HasInboundLinks = true
			collectionsObjectsIds = append(collectionsObjectsIds, file.PageID)
		}
	}

	return collectionsObjectsIds
}

func (m *Markdown) processFieldBlockIfItIs(blocks []*model.Block, files *fileContainer) (blocksOut []*model.Block) {
	if len(blocks) < 1 || blocks[0].GetText() == nil {
		return blocks
	}
	blocksOut = blocks

	txt := blocks[0].GetText().Text
	if txt == "" ||
		(blocks[0].GetText().Marks != nil && len(blocks[0].GetText().Marks.Marks) > 0) {
		// fields can't have a markup
		return blocks
	}

	potentialPairs := strings.Split(txt, "\n")

	var text string
	var marks []*model.BlockContentTextMark
	for _, pair := range potentialPairs {
		if text != "" {
			text += "\n"
		}
		if len(pair) <= 3 || pair[len(pair)-3:] != ".md" {
			text += pair
			continue
		}

		keyVal := strings.SplitN(pair, ":", 2)
		if len(keyVal) < 2 {
			text += pair
			continue
		}

		potentialFileNames := strings.Split(keyVal[1], ",")
		text += keyVal[0] + ": "

		for potentialFileNameIndex, potentialFileName := range potentialFileNames {
			potentialFileName, _ = url.PathUnescape(potentialFileName)
			potentialFileName = strings.ReplaceAll(potentialFileName, `"`, "")
			if potentialFileNameIndex != 0 {
				text += ", "
			}
			shortPath := ""
			id := m.getIdFromPath(potentialFileName)
			for name, _ := range files.byPath {
				if m.getIdFromPath(name) == id {
					shortPath = name
					break
				}
			}

			file := findFile(files, shortPath)

			if file == nil || len(file.PageID) == 0 {
				text += potentialFileName
				log.Errorf("target file not found")
			} else {
				log.Debug("target file found:", file.PageID)
				file.HasInboundLinks = true
				if file.Title == "" {
					// shouldn't be a case
					file.Title = shortPath
				}

				text += file.Title
				marks = append(marks, &model.BlockContentTextMark{
					Range: &model.Range{int32(len(text) - len(file.Title)), int32(len(text))},
					Type:  model.BlockContentTextMark_Mention,
					Param: file.PageID,
				})
			}
		}
	}

	if len(marks) > 0 {
		blockText := blocks[0].GetText()
		blockText.Text = text
		blockText.Marks = &model.BlockContentTextMarks{Marks: marks}
	}

	return blocksOut
}

func (m *Markdown) getIdFromPath(path string) (id string) {
	a := strings.Split(path, " ")
	b := a[len(a)-1]
	if len(b) < 3 {
		return ""
	}
	return b[:len(b)-3]
}

func (m *Markdown) setInboundLinks(files *fileContainer, progress process.Progress, _ map[string]*domain.Details, allErrors *common.ConvertError) {
	progress.SetProgressMessage("Start linking database file with pages")
	for name := range files.byPath {
		if err := progress.TryStep(1); err != nil {
			allErrors.Add(common.ErrCancel)
			return
		}

		if !strings.EqualFold(filepath.Ext(name), ".csv") {
			continue
		}

		ext := filepath.Ext(name)
		csvDir := strings.TrimSuffix(name, ext)

		for targetName, targetFile := range files.byPath {
			fileExt := filepath.Ext(targetName)
			if filepath.Dir(targetName) == csvDir && strings.EqualFold(fileExt, ".md") {
				targetFile.HasInboundLinks = true
			}
		}
	}
}

func (m *Markdown) linkPagesWithRootFile(files *fileContainer, progress process.Progress, _ map[string]*domain.Details, allErrors *common.ConvertError) {
	progress.SetProgressMessage("Start linking database with pages")
	for name, file := range files.byPath {
		if err := progress.TryStep(1); err != nil {
			allErrors.Add(common.ErrCancel)
			return
		}

		if file.PageID == "" {
			// file is not a page
			continue
		}

		if strings.EqualFold(filepath.Ext(name), ".csv") {
			file.CollectionsObjectsIds = m.retrieveCollectionObjectsIds(name, files)
		}

		blocks := make([]*model.Block, 0, len(file.ParsedBlocks))

		for i, b := range file.ParsedBlocks {
			if f := b.GetFile(); f != nil && strings.EqualFold(filepath.Ext(f.Name), ".csv") {
				csvFile, exists := files.byPath[f.Name]
				if !exists {
					continue
				}
				csvFile.HasInboundLinks = true
				csvFile.CollectionsObjectsIds = m.retrieveCollectionObjectsIds(f.Name, files)

				blocks = append(blocks, m.getLinkBlock(file))
			} else {
				blocks = append(blocks, file.ParsedBlocks[i])
			}
		}
		file.ParsedBlocks = blocks
	}
}

func (m *Markdown) getLinkBlock(file *FileInfo) *model.Block {
	fields := make(map[string]*types.Value)
	fields[bundle.RelationKeyName.String()] = &types.Value{
		Kind: &types.Value_StringValue{StringValue: file.Title},
	}
	return &model.Block{
		Id: bson.NewObjectId().Hex(),
		Content: &model.BlockContentOfLink{
			Link: &model.BlockContentLink{
				TargetBlockId: file.PageID,
				Style:         model.BlockContentLink_Page,
				Fields: &types.Struct{
					Fields: fields,
				},
			},
		},
	}
}

func (m *Markdown) addLinkBlocks(files *fileContainer, progress process.Progress, _ map[string]*domain.Details, allErrors *common.ConvertError) {
	progress.SetProgressMessage("Start creating link blocks")
	for _, file := range files.byPath {
		if err := progress.TryStep(1); err != nil {
			allErrors.Add(common.ErrCancel)
			return
		}

		if file.PageID == "" {
			// not a page
			continue
		}

		if file.HasInboundLinks {
			continue
		}

		file.ParsedBlocks = append(file.ParsedBlocks, &model.Block{
			Content: &model.BlockContentOfLink{
				Link: &model.BlockContentLink{
					TargetBlockId: file.PageID,
					Style:         model.BlockContentLink_Page,
					Fields:        nil,
				},
			},
		})
	}
}

func bundledRelationLinks(relationDetails *domain.Details) []*model.RelationLink {
	links := make([]*model.RelationLink, 0, relationDetails.Len())
	relationDetails.Iterate()(func(key domain.RelationKey, value domain.Value) bool {
		rel, err := bundle.GetRelation(key)
		if err != nil {
			log.Warnf("relation %s not found in bundle", key)
			return true
		}

		links = append(links, &model.RelationLink{
			Key:    rel.Key,
			Format: rel.Format,
		})
		return true
	})
	return links
}

func (m *Markdown) createSnapshots(
	req *pb.RpcObjectImportRequest,
	pathsCount int,
	files *fileContainer,
	progress process.Progress,
	details map[string]*domain.Details,
	allErrors *common.ConvertError,
) []*common.Snapshot {
	snapshots := make([]*common.Snapshot, 0)
	relationsSnapshots := make([]*common.Snapshot, 0)
	objectTypeSnapshots := make([]*common.Snapshot, 0)
	progress.SetProgressMessage("Start creating snapshots")

	// Check if we have schemas loaded
	hasSchemas := m.blockConverter.schemaImporter.HasSchemas()

	// First pass: collect all YAML properties to create relation snapshots
	yamlRelations := make(map[string]*yaml.Property)          // property name -> property
	yamlRelationOptions := make(map[string]map[string]string) // relationKey -> optionValue -> optionId
	objectTypes := make(map[string][]string)                  // Track unique object type names

	for _, file := range files.byPath {
		var props = make([]string, 0, len(file.YAMLProperties))
		if file.YAMLProperties != nil {
			for i := range file.YAMLProperties {
				prop := &file.YAMLProperties[i]

				// Schema resolution already happened during YAML parsing if schemas were available

				// Use existing relation if already seen
				if _, exists := yamlRelations[prop.Name]; !exists {
					yamlRelations[prop.Name] = prop
				}
				props = append(props, yamlRelations[prop.Name].Key)

				// Collect option values for non-schema imports
				if !hasSchemas && (prop.Format == model.RelationFormat_status || prop.Format == model.RelationFormat_tag) {
					if yamlRelationOptions[prop.Key] == nil {
						yamlRelationOptions[prop.Key] = make(map[string]string)
					}

					// Collect values
					switch prop.Format {
					case model.RelationFormat_status:
						if val := prop.Value.String(); val != "" {
							yamlRelationOptions[prop.Key][val] = ""
						}
					case model.RelationFormat_tag:
						for _, val := range prop.Value.StringList() {
							yamlRelationOptions[prop.Key][val] = ""
						}
					}
				}
			}
		}

		// Collect object types
		if file.ObjectTypeName != "" {
			objectTypes[file.ObjectTypeName] = props
		}
	}

	// Variable to hold objectTypeKeys
	var objectTypeKeys map[string]string

	// If we have schemas, use them to create relations and types
	if hasSchemas {
		// Create relation snapshots from schemas
		relationsSnapshots = append(relationsSnapshots, m.blockConverter.schemaImporter.CreateRelationSnapshots()...)

		// Create relation option snapshots from schemas
		relationsSnapshots = append(relationsSnapshots, m.blockConverter.schemaImporter.CreateRelationOptionSnapshots()...)

		// Create type snapshots from schemas
		schemaTypeSnapshots := m.blockConverter.schemaImporter.CreateTypeSnapshots()
		objectTypeSnapshots = append(objectTypeSnapshots, schemaTypeSnapshots...)

		// Map type names to IDs for later use
		objectTypeKeys = make(map[string]string)

		// First, add all schema-defined types
		for _, snapshot := range schemaTypeSnapshots {
			if snapshot.Snapshot != nil && snapshot.Snapshot.Data != nil {
				typeName := snapshot.Snapshot.Data.Details.GetString(bundle.RelationKeyName)
				typeKey := snapshot.Snapshot.Data.Key
				if typeName != "" && typeKey != "" {
					objectTypeKeys[typeName] = typeKey
				}
			}
		}

		// Then check for any types used in YAML that aren't in schemas and create them
		for typeName, props := range objectTypes {
			if _, exists := objectTypeKeys[typeName]; !exists {
				// This type was referenced but not defined in schemas, create it
				typeKey := bson.NewObjectId().Hex()
				objectTypeKeys[typeName] = typeKey

				// Create object type snapshot
				props := append([]string{bundle.TypeKeyObjectType.String()}, props...)
				objectTypeDetails := getObjectTypeDetails(typeName, typeKey, props)
				objectTypeSnapshots = append(objectTypeSnapshots, &common.Snapshot{
					Id: typeIdPrefix + typeKey,
					Snapshot: &common.SnapshotModel{
						SbType: smartblock.SmartBlockTypeObjectType,
						Data: &common.StateSnapshot{
							Details:       objectTypeDetails,
							RelationLinks: bundledRelationLinks(objectTypeDetails),
							ObjectTypes:   []string{bundle.TypeKeyObjectType.String()},
							Key:           typeKey,
						},
					},
				})

				log.Debugf("Created type '%s' with key '%s' (referenced in YAML but not found in schemas)", typeName, typeKey)
			}
		}
	} else {
		// Fallback to original YAML-based creation
		// Create relation snapshots for YAML properties
		for propName, prop := range yamlRelations {
			// Generate BSON ID for the relation key
			relationDetails := getRelationDetails(propName, prop.Key, float64(prop.Format), prop.IncludeTime)

			relationsSnapshots = append(relationsSnapshots, &common.Snapshot{
				Id: propIdPrefix + prop.Key,
				Snapshot: &common.SnapshotModel{
					SbType: smartblock.SmartBlockTypeRelation,
					Data: &common.StateSnapshot{
						Details:       relationDetails,
						RelationLinks: bundledRelationLinks(relationDetails),
						ObjectTypes:   []string{bundle.TypeKeyRelation.String()},
						Key:           prop.Key,
					},
				},
			})
		}

		typeRelation := bundle.MustGetRelation(bundle.RelationKeyType)
		details := getRelationDetails(typeRelation.Name, typeRelation.Key, float64(typeRelation.Format), false)
		relationsSnapshots = append(relationsSnapshots, &common.Snapshot{
			Id: propIdPrefix + bundle.TypeKeyObjectType.String(),
			Snapshot: &common.SnapshotModel{
				SbType: smartblock.SmartBlockTypeRelation,
				Data: &common.StateSnapshot{
					Details:       details,
					ObjectTypes:   []string{bundle.TypeKeyRelation.String()},
					Key:           bundle.TypeKeyObjectType.String(),
					RelationLinks: bundledRelationLinks(details),
				},
			},
		})

		// Create object type snapshots for YAML type values
		objectTypeKeys = make(map[string]string) // name -> key mapping
		for typeName := range objectTypes {
			typeKey := bson.NewObjectId().Hex()
			// Create object type snapshot
			props := append([]string{bundle.TypeKeyObjectType.String()}, objectTypes[typeName]...)
			objectTypeDetails := getObjectTypeDetails(typeName, typeKey, props)
			objectTypeSnapshots = append(objectTypeSnapshots, &common.Snapshot{
				Id: typeIdPrefix + typeKey,
				Snapshot: &common.SnapshotModel{
					SbType: smartblock.SmartBlockTypeObjectType,
					Data: &common.StateSnapshot{
						Details:       objectTypeDetails,
						RelationLinks: bundledRelationLinks(objectTypeDetails),
						ObjectTypes:   []string{bundle.TypeKeyObjectType.String()},
						Key:           typeKey,
					},
				},
			})
			objectTypeKeys[typeName] = typeKey
		}

		// Create relation option snapshots for YAML values
		for relationKey, options := range yamlRelationOptions {
			for optionValue := range options {
				optionId := m.blockConverter.schemaImporter.optionId(relationKey, optionValue)
				yamlRelationOptions[relationKey][optionValue] = optionId

				optionDetails := domain.NewDetails()
				optionDetails.SetString(bundle.RelationKeyRelationKey, relationKey)
				optionDetails.SetString(bundle.RelationKeyName, optionValue)
				optionDetails.SetInt64(bundle.RelationKeyLayout, int64(model.ObjectType_relationOption))
				optionDetails.SetString(bundle.RelationKeyRelationOptionColor, constant.RandomOptionColor().String())
				// Set unique key for the option
				optionKey := fmt.Sprintf("%s_%s", relationKey, optionValue)
				uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelationOption, optionKey)
				if err != nil {
					log.Errorf("failed to create unique key for relation option '%s': %v", optionKey, err)
					continue
				}
				optionDetails.SetString(bundle.RelationKeyUniqueKey, uniqueKey.Marshal())

				relationsSnapshots = append(relationsSnapshots, &common.Snapshot{
					Id: optionId,
					Snapshot: &common.SnapshotModel{
						SbType: smartblock.SmartBlockTypeRelationOption,
						Data: &common.StateSnapshot{
							Details:     optionDetails,
							ObjectTypes: []string{bundle.TypeKeyRelationOption.String()},
						},
					},
				})
			}
		}
	}

	// Fix YAML details to use option IDs for non-schema imports
	if !hasSchemas {
		for filePath, d := range details {
			if details != nil {
				// Create a new details object with updated values
				updatedDetails := domain.NewDetails()
				d.Iterate()(func(key domain.RelationKey, value domain.Value) bool {
					var propFormat model.RelationFormat
					for _, prop := range files.byPath[filePath].YAMLProperties {
						if prop.Key == string(key) {
							propFormat = prop.Format
							break
						}
					}

					// Update the value to use option IDs
					switch propFormat {
					case model.RelationFormat_status, model.RelationFormat_tag:
						list := value.WrapToStringList()
						for i := range list {
							list[i] = m.blockConverter.schemaImporter.optionId(key.String(), list[i])
						}
						updatedDetails.Set(key, domain.StringList(list))
					default:
						// For other formats, just copy the value as is
						// Copy unchanged values
						updatedDetails.Set(key, value)
					}
					return true
				})
				details[filePath] = updatedDetails
			}
		}
	}

	// Second pass: create object snapshots
	for name, file := range files.byPath {
		if err := progress.TryStep(1); err != nil {
			allErrors.Add(common.ErrCancel)
			return nil
		}

		if file.PageID == "" {
			// file is not a page
			continue
		}

		if filepath.Ext(name) == ".csv" {
			var err error
			snapshots, err = m.addCollectionSnapshot(name, file, snapshots)
			if err != nil {
				allErrors.Add(err)
				if allErrors.ShouldAbortImport(pathsCount, model.Import_Markdown) {
					return nil
				}
			}
			continue
		}

		// Add relation links for YAML properties
		var relationLinks []*model.RelationLink
		if file.YAMLProperties != nil {
			for _, prop := range file.YAMLProperties {
				relationLinks = append(relationLinks, &model.RelationLink{
					Key:    prop.Key,
					Format: prop.Format,
				})
			}
		}

		// Determine object type
		objectTypeKey := bundle.TypeKeyPage.String()
		isCollectionType := false
		if file.ObjectTypeName != "" {
			if typeKey, exists := objectTypeKeys[file.ObjectTypeName]; exists {
				objectTypeKey = typeKey
			} else {
				// Try to find bundled type by name
				bundledKey, err := bundle.GetTypeKeyByName(file.ObjectTypeName)
				if err == nil {
					objectTypeKey = bundledKey.String()
				}
			}

			// Check if this type is a collection type from schema
			if m.blockConverter.schemaImporter != nil {
				for _, s := range m.blockConverter.schemaImporter.GetSchemas() {
					if s.Type != nil && s.Type.Name == file.ObjectTypeName && s.Type.Layout == model.ObjectType_collection {
						isCollectionType = true
						break
					}
				}
			}
		}

		// Check if YAML has Collection property
		var collectionObjectIds []string
		if file.YAMLProperties != nil {
			// Look for Collection property in the parsed properties
			for _, prop := range file.YAMLProperties {
				// Check both by key and by name (case-insensitive)
				if prop.Key == schema.CollectionPropertyKey ||
					strings.EqualFold(prop.Name, "Collection") {
					isCollectionType = true
					collectionObjectIds = prop.Value.WrapToStringList()
					break
				}
			}
		}

		// Collections are still pages, just with collection layout
		sbType := smartblock.SmartBlockTypePage

		blocks := file.ParsedBlocks

		// Create state snapshot
		stateSnapshot := &common.StateSnapshot{
			Blocks:        blocks,
			Details:       details[name],
			ObjectTypes:   []string{objectTypeKey},
			RelationLinks: relationLinks,
		}

		// Add collection store if this is a collection
		if isCollectionType && len(collectionObjectIds) > 0 {
			// Resolve collection object references to page IDs
			resolvedIds := make([]string, 0, len(collectionObjectIds))
			collectionDir := filepath.Dir(name) // Directory where the collection file is located

			for _, ref := range collectionObjectIds {
				// ref should be a filename path (relative or absolute) with .md suffix
				// Normalize the reference path
				refPath := ref
				if !strings.HasSuffix(refPath, ".md") {
					refPath = refPath + ".md"
				}

				found := false
				for fileName, f := range files.byPath {
					if f.PageID == "" {
						continue
					}

					// Try different matching strategies:
					// 1. Exact match (for absolute paths or paths relative to import root)
					if fileName == refPath {
						resolvedIds = append(resolvedIds, f.PageID)
						found = true
						break
					}

					// 2. Match relative to collection file's directory
					relativeToCollection := filepath.Join(collectionDir, refPath)
					if fileName == relativeToCollection {
						resolvedIds = append(resolvedIds, f.PageID)
						found = true
						break
					}

					// 3. Match by base filename only (fallback for simple filenames)
					if filepath.Base(fileName) == filepath.Base(refPath) {
						resolvedIds = append(resolvedIds, f.PageID)
						found = true
						break
					}
				}
				if !found {
					log.Warnf("Collection reference '%s' not found in import", ref)
				}
			}

			if len(resolvedIds) > 0 {
				// Set collection store using Collections field
				stateSnapshot.Collections = &types.Struct{
					Fields: map[string]*types.Value{
						template.CollectionStoreKey: pbtypes.StringList(resolvedIds),
					},
				}
			}
		}

		snapshots = append(snapshots, &common.Snapshot{
			Id:       file.PageID,
			FileName: name,
			Snapshot: &common.SnapshotModel{
				SbType: sbType,
				Data:   stateSnapshot,
			},
		})
	}

	// Append relation snapshots at the end
	snapshots = append(snapshots, relationsSnapshots...)
	// Append object type snapshots at the end
	snapshots = append(snapshots, objectTypeSnapshots...)

	return snapshots
}

func (m *Markdown) addCollectionSnapshot(fileName string, file *FileInfo, snapshots []*common.Snapshot) ([]*common.Snapshot, error) {
	c := common.NewImportCollection(m.service)
	settings := common.NewImportCollectionSetting(
		common.WithCollectionName(file.Title),
		common.WithTargetObjects(file.CollectionsObjectsIds),
	)
	csvCollection, err := c.MakeImportCollection(settings)
	if err != nil {
		return nil, err
	}
	csvCollection.Id = file.PageID
	csvCollection.FileName = fileName
	snapshots = append(snapshots, csvCollection)
	return snapshots, nil
}

func (m *Markdown) addPropertyBlocks(files *fileContainer, progress process.Progress, details map[string]*domain.Details, allErrors *common.ConvertError) {
	for _, file := range files.byPath {
		// Create relation blocks for properties (excluding system properties)
		propertyBlocks := m.createPropertyBlocks(file.YAMLProperties)
		if len(propertyBlocks) > 0 {
			// Insert property blocks at the beginning
			file.ParsedBlocks = append(propertyBlocks, file.ParsedBlocks...)
		}
	}
}

func (m *Markdown) addChildBlocks(files *fileContainer, progress process.Progress, _ map[string]*domain.Details, allErrors *common.ConvertError) {
	progress.SetProgressMessage("Start creating root blocks")
	childBlocks := m.extractChildBlocks(files)
	for _, file := range files.byPath {
		if err := progress.TryStep(1); err != nil {
			allErrors.Add(common.ErrCancel)
			return
		}

		if file.PageID == "" {
			// not a page
			continue
		}

		childrenIds := make([]string, 0, len(file.ParsedBlocks))
		for _, b := range file.ParsedBlocks {
			if isChildBlock(childBlocks, b) {
				continue
			}
			childrenIds = append(childrenIds, b.Id)
		}

		file.ParsedBlocks = append(file.ParsedBlocks, &model.Block{
			Id:          file.PageID,
			ChildrenIds: childrenIds,
			Content: &model.BlockContentOfSmartblock{
				Smartblock: &model.BlockContentSmartblock{},
			},
		})
	}
}

func (m *Markdown) extractChildBlocks(files *fileContainer) map[string]struct{} {
	childBlocks := make(map[string]struct{})
	for _, file := range files.byPath {
		if file.PageID == "" {
			continue
		}

		for _, b := range file.ParsedBlocks {
			for _, childBlock := range b.ChildrenIds {
				childBlocks[childBlock] = struct{}{}
			}
		}
	}
	return childBlocks
}

func (m *Markdown) addLinkToObjectBlocks(files *fileContainer, progress process.Progress, _ map[string]*domain.Details, allErrors *common.ConvertError) {
	progress.SetProgressMessage("Start linking blocks")
	for _, file := range files.byPath {
		if err := progress.TryStep(1); err != nil {
			allErrors.Add(common.ErrCancel)
			return
		}

		if file.PageID == "" {
			// file is not a page
			continue
		}

		file.ParsedBlocks = m.processFieldBlockIfItIs(file.ParsedBlocks, files)

		for _, block := range file.ParsedBlocks {
			if link := block.GetLink(); link != nil {
				target, err := url.PathUnescape(normalizePath(link.TargetBlockId))
				if err != nil {
					log.Warnf("error while url.PathUnescape: %s", err)
					target = link.TargetBlockId
				}

				if file := findFile(files, target); file != nil {
					link.TargetBlockId = file.PageID
					file.HasInboundLinks = true
				}

				continue
			}

			if text := block.GetText(); text != nil && text.Marks != nil && len(text.Marks.Marks) > 0 {
				for _, mark := range text.Marks.Marks {
					if mark.Type != model.BlockContentTextMark_Mention && mark.Type != model.BlockContentTextMark_Object {
						continue
					}

					if targetFile := findFile(files, normalizePath(mark.Param)); targetFile != nil {
						mark.Param = targetFile.PageID
						targetFile.HasInboundLinks = true
					}
				}
			}
		}
	}
}

func (m *Markdown) fillEmptyBlocks(files *fileContainer, progress process.Progress, _ map[string]*domain.Details, allErrors *common.ConvertError) {
	progress.SetProgressMessage("Start creating file blocks")
	// process file blocks
	for _, file := range files.byPath {
		if err := progress.TryStep(1); err != nil {
			allErrors.Add(common.ErrCancel)
			return
		}

		if file.PageID == "" {
			continue
		}

		for _, b := range file.ParsedBlocks {
			if b.Id == "" {
				b.Id = bson.NewObjectId().Hex()
			}
		}
	}
}

func (m *Markdown) setNewID(files *fileContainer, progress process.Progress, _ map[string]*domain.Details, allErrors *common.ConvertError) {
	progress.SetProgressMessage("Start creating blocks")
	for name, file := range files.byName {
		if err := progress.TryStep(1); err != nil {
			allErrors.Add(common.ErrCancel)
			return
		}

		// Assign PageID to markdown files, CSV files, and directory pages
		ext := strings.ToLower(filepath.Ext(name))
		if ext == ".md" || ext == ".csv" || ext == "" {
			file.PageID = bson.NewObjectId().Hex()
		}
	}
}

func (m *Markdown) setDetails(file *FileInfo, fileName string, details map[string]*domain.Details) {
	var title, emoji string
	if len(file.ParsedBlocks) > 0 {
		title, emoji = m.extractTitleAndEmojiFromBlock(file)
	}
	details[fileName] = common.GetCommonDetails(fileName, title, emoji, model.ObjectType_basic)

	// Set YAML details directly
	if file.YAMLDetails != nil {
		file.YAMLDetails.Iterate()(func(key domain.RelationKey, value domain.Value) bool {
			details[fileName].Set(key, value)
			return true
		})
	}

	file.Title = details[fileName].GetString(bundle.RelationKeyName)
}

func (m *Markdown) extractTitleAndEmojiFromBlock(file *FileInfo) (string, string) {
	var title, emoji string
	if text := file.ParsedBlocks[0].GetText(); text != nil && text.Style == model.BlockContentText_Header1 {
		title = text.Text
		titleParts := strings.SplitN(title, " ", 2)

		// only select the first rune to see if it looks like emoji
		if len(titleParts) == 2 && emojiAproxRegexp.MatchString(string([]rune(titleParts[0])[0:1])) {
			// first symbol is emoji - just use it all before the space
			emoji = titleParts[0]
			title = titleParts[1]
		}
		// remove title block
		file.ParsedBlocks = file.ParsedBlocks[1:]
	}

	return title, emoji
}

func (m *Markdown) retrieveRootObjectsIds(files *fileContainer) []string {
	var rootObjectsIds []string
	for path, file := range files.byPath {
		if file.PageID == "" {
			continue
		}
		if file.IsRootFile {
			rootObjectsIds = append(rootObjectsIds, file.PageID)
		}
		// Also include top-level directory pages
		dir := filepath.Dir(path)
		if dir == "." || dir == "/" {
			// This is a top-level item (file or directory page)
			rootObjectsIds = append(rootObjectsIds, file.PageID)
		}
	}
	return rootObjectsIds
}

func isChildBlock(blocks map[string]struct{}, b *model.Block) bool {
	_, ok := blocks[b.Id]
	return ok
}

func getRelationDetails(name, key string, format float64, includeTime bool) *domain.Details {
	details := domain.NewDetails()
	details.SetFloat64(bundle.RelationKeyRelationFormat, format)
	details.SetString(bundle.RelationKeyName, name)
	details.SetString(bundle.RelationKeyRelationKey, key)
	details.SetInt64(bundle.RelationKeyLayout, int64(model.ObjectType_relation))

	// Set includeTime for date relations
	if format == float64(model.RelationFormat_date) {
		details.SetBool(bundle.RelationKeyRelationFormatIncludeTime, includeTime)
	}

	uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, key)
	if err != nil {
		log.Warnf("failed to create unique key for YAML relation: %v", err)
		return details
	}
	details.SetString(bundle.RelationKeyId, uniqueKey.Marshal())
	return details
}

func propKeysToIds(propKeys []string) []string {
	ids := make([]string, len(propKeys))
	for i, key := range propKeys {
		ids[i] = propIdPrefix + key
	}
	return ids
}
func getObjectTypeDetails(name, key string, propKeys []string) *domain.Details {
	details := domain.NewDetails()
	details.SetString(bundle.RelationKeyName, name)
	details.SetInt64(bundle.RelationKeyLayout, int64(model.ObjectType_objectType))
	details.SetInt64(bundle.RelationKeyResolvedLayout, int64(model.ObjectType_objectType))
	details.SetInt64(bundle.RelationKeyRecommendedLayout, int64(model.ObjectType_basic))
	details.SetString(bundle.RelationKeyType, bundle.TypeKeyObjectType.String())

	propIds := propKeysToIds(propKeys)
	// first 3 goes to featured relations, the rest to properties
	maxFeaturedRelations := 3
	if len(propIds) < maxFeaturedRelations {
		maxFeaturedRelations = len(propIds)
	}
	details.SetStringList(bundle.RelationKeyRecommendedFeaturedRelations, propIds[:maxFeaturedRelations])
	if len(propKeys) > maxFeaturedRelations {
		details.SetStringList(bundle.RelationKeyRecommendedRelations, propIds[maxFeaturedRelations:])
	}

	// Create unique key for the object type
	uniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, key)
	if err != nil {
		log.Warnf("failed to create unique key for YAML object type: %v", err)
		return details
	}
	details.SetString(bundle.RelationKeyId, uniqueKey.Marshal())
	details.SetString(bundle.RelationKeyUniqueKey, uniqueKey.Marshal())

	return details
}

func (m *Markdown) findFileByPath(path string, files *fileContainer) *FileInfo {
	// First try exact match
	if file, exists := files.byPath[path]; exists {
		return file
	}

	// If not found, try to match by comparing paths
	name := filepath.Base(path)
	if file, exists := files.byName[name]; exists {
		return file
	}
	return nil
}

func (m *Markdown) processObjectProperties(files *fileContainer, progress process.Progress, details map[string]*domain.Details, allErrors *common.ConvertError) {
	progress.SetProgressMessage("Start linking blocks")

	for fileName, file := range files.byPath {
		if err := progress.TryStep(1); err != nil {
			allErrors.Add(common.ErrCancel)
			return
		}

		for i := range file.YAMLProperties {
			prop := &file.YAMLProperties[i]
			if prop.Format == model.RelationFormat_object {
				vals := file.YAMLDetails.Get(domain.RelationKey(prop.Key))
				paths := vals.WrapToStringList()
				ids := make([]string, 0, len(paths))
				for _, path := range paths {
					// The path should already be absolute from YAML parsing
					// Find the file in the files map
					targetFile := m.findFileByPath(path, files)
					if targetFile != nil && targetFile.PageID != "" {
						ids = append(ids, targetFile.PageID)
					}
				}
				file.YAMLDetails.Set(domain.RelationKey(prop.Key), domain.StringList(ids))
			}
		}
		m.setDetails(file, fileName, details)
	}
}

// createPropertyBlocks creates relation blocks for YAML properties
func (m *Markdown) createPropertyBlocks(properties []yaml.Property) []*model.Block {
	var blocks []*model.Block

	// Define system properties to exclude
	systemProperties := map[string]bool{
		bundle.RelationKeyName.String():             true,
		bundle.RelationKeyDescription.String():      true,
		bundle.RelationKeyType.String():             true,
		bundle.RelationKeyCreatedDate.String():      true,
		bundle.RelationKeyLastModifiedDate.String(): true,
		bundle.RelationKeyCreator.String():          true,
		bundle.RelationKeyLastModifiedBy.String():   true,
		bundle.RelationKeyId.String():               true,
		bundle.RelationKeyIconEmoji.String():        true,
		bundle.RelationKeyIconImage.String():        true,
		bundle.RelationKeyCoverId.String():          true,
		bundle.RelationKeyCoverType.String():        true,
		bundle.RelationKeyCoverX.String():           true,
		bundle.RelationKeyCoverY.String():           true,
		bundle.RelationKeyCoverScale.String():       true,
		bundle.RelationKeyLayout.String():           true,
		bundle.RelationKeyLayoutAlign.String():      true,
	}

	for _, prop := range properties {
		// Skip system properties
		if systemProperties[prop.Key] {
			continue
		}

		// Create a relation block
		block := &model.Block{
			Id: bson.NewObjectId().Hex(),
			Content: &model.BlockContentOfRelation{
				Relation: &model.BlockContentRelation{
					Key: prop.Key,
				},
			},
		}
		blocks = append(blocks, block)
	}

	return blocks
}
