package markdown

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/collection"
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
)

var (
	emojiAproxRegexp = regexp.MustCompile(`[\x{2194}-\x{329F}\x{1F000}-\x{1FADF}]`)
	log              = logging.Logger("markdown-import")
)

const numberOfStages = 9 // 8 cycles to get snapshots and 1 cycle to create objects

type Markdown struct {
	blockConverter *mdConverter
	service        *collection.Service
	schemas        map[string]*schema.Schema // typeName -> Schema
}

const (
	Name               = "Markdown"
	rootCollectionName = "Markdown Import"
	propIdPrefix       = "id_prop"
	typeIdPrefix       = "id_type"
)

func New(tempDirProvider core.TempDirProvider, service *collection.Service) common.Converter {
	return &Markdown{
		blockConverter: newMDConverter(tempDirProvider),
		service:        service,
		schemas:        make(map[string]*schema.Schema),
	}
}

func (m *Markdown) Name() string {
	return Name
}

func (m *Markdown) GetParams(req *pb.RpcObjectImportRequest) []string {
	if p := req.GetMarkdownParams(); p != nil {
		return p.Path
	}

	return nil
}

func (m *Markdown) GetImage() ([]byte, int64, int64, error) {
	return nil, 0, 0, nil
}

func (m *Markdown) GetSnapshots(ctx context.Context, req *pb.RpcObjectImportRequest, progress process.Progress) (*common.Response, *common.ConvertError) {
	paths := m.GetParams(req)
	if len(paths) == 0 {
		return nil, nil
	}
	allErrors := common.NewError(req.Mode)
	allSnapshots, allRootObjectsIds := m.processFiles(req, progress, paths, allErrors)
	if allErrors.ShouldAbortImport(len(paths), req.Type) {
		return nil, allErrors
	}
	allSnapshots, rootCollectionID, err := m.createRootCollection(allSnapshots, allRootObjectsIds)
	if err != nil {
		allErrors.Add(err)
		if allErrors.ShouldAbortImport(len(paths), req.Type) {
			return nil, allErrors
		}
	}

	if allErrors.IsEmpty() {
		return &common.Response{Snapshots: allSnapshots, RootCollectionID: rootCollectionID}, nil
	}
	return &common.Response{Snapshots: allSnapshots, RootCollectionID: rootCollectionID}, allErrors
}

func (m *Markdown) processFiles(req *pb.RpcObjectImportRequest, progress process.Progress, paths []string, allErrors *common.ConvertError) ([]*common.Snapshot, []string) {
	var (
		allSnapshots      []*common.Snapshot
		allRootObjectsIds []string
	)
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

func (m *Markdown) getSnapshotsAndRootObjectsIds(
	req *pb.RpcObjectImportRequest,
	progress process.Progress,
	path string,
	allErrors *common.ConvertError,
) ([]*common.Snapshot, []string) {
	importSource := source.GetSource(path)
	if importSource == nil {
		return nil, nil
	}
	defer importSource.Close()
	err := importSource.Initialize(path)
	if err != nil {
		allErrors.Add(err)
		if allErrors.ShouldAbortImport(0, model.Import_Markdown) {
			return nil, nil
		}
	}
	// Load schemas if available
	if err := m.loadSchemas(importSource, allErrors); err != nil {
		log.Warnf("failed to load schemas: %v", err)
	}

	files := m.blockConverter.markdownToBlocks(path, importSource, allErrors)
	pathsCount := len(req.GetMarkdownParams().Path)
	if allErrors.ShouldAbortImport(pathsCount, req.Type) {
		return nil, nil
	}

	progress.SetTotal(int64(numberOfStages * len(files)))
	details := make(map[string]*domain.Details, 0)

	if m.processImportStep(pathsCount, files, progress, allErrors, details, m.setInboundLinks) ||
		m.processImportStep(pathsCount, files, progress, allErrors, details, m.setNewID) ||
		m.processImportStep(pathsCount, files, progress, allErrors, details, m.addLinkToObjectBlocks) ||
		m.processImportStep(pathsCount, files, progress, allErrors, details, m.linkPagesWithRootFile) ||
		m.processImportStep(pathsCount, files, progress, allErrors, details, m.addLinkBlocks) ||
		m.processImportStep(pathsCount, files, progress, allErrors, details, m.fillEmptyBlocks) ||
		m.processImportStep(pathsCount, files, progress, allErrors, details, m.addChildBlocks) {
		return nil, nil
	}

	return m.createSnapshots(pathsCount, files, progress, details, allErrors), m.retrieveRootObjectsIds(files)
}

func (m *Markdown) processImportStep(pathCount int,
	files map[string]*FileInfo,
	progress process.Progress,
	allErrors *common.ConvertError,
	details map[string]*domain.Details,
	callback func(map[string]*FileInfo, process.Progress, map[string]*domain.Details, *common.ConvertError),
) (abortImport bool) {
	callback(files, progress, details, allErrors)
	return allErrors.ShouldAbortImport(pathCount, model.Import_Markdown)
}

func (m *Markdown) retrieveCollectionObjectsIds(csvFileName string, files map[string]*FileInfo) []string {
	ext := filepath.Ext(csvFileName)
	csvDir := strings.TrimSuffix(csvFileName, ext)
	var collectionsObjectsIds []string
	for name, file := range files {
		fileExt := filepath.Ext(name)
		if filepath.Dir(name) == csvDir && strings.EqualFold(fileExt, ".md") {
			file.HasInboundLinks = true
			collectionsObjectsIds = append(collectionsObjectsIds, file.PageID)
		}
	}

	return collectionsObjectsIds
}

func (m *Markdown) processFieldBlockIfItIs(blocks []*model.Block, files map[string]*FileInfo) (blocksOut []*model.Block) {
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
			for name, _ := range files {
				if m.getIdFromPath(name) == id {
					shortPath = name
					break
				}
			}

			file := files[shortPath]

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

func (m *Markdown) setInboundLinks(files map[string]*FileInfo, progress process.Progress, _ map[string]*domain.Details, allErrors *common.ConvertError) {
	progress.SetProgressMessage("Start linking database file with pages")
	for name := range files {
		if err := progress.TryStep(1); err != nil {
			allErrors.Add(common.ErrCancel)
			return
		}

		if !strings.EqualFold(filepath.Ext(name), ".csv") {
			continue
		}

		ext := filepath.Ext(name)
		csvDir := strings.TrimSuffix(name, ext)

		for targetName, targetFile := range files {
			fileExt := filepath.Ext(targetName)
			if filepath.Dir(targetName) == csvDir && strings.EqualFold(fileExt, ".md") {
				targetFile.HasInboundLinks = true
			}
		}
	}
}

func (m *Markdown) linkPagesWithRootFile(files map[string]*FileInfo, progress process.Progress, _ map[string]*domain.Details, allErrors *common.ConvertError) {
	progress.SetProgressMessage("Start linking database with pages")
	for name, file := range files {
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
				csvFile, exists := files[f.Name]
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

func (m *Markdown) addLinkBlocks(files map[string]*FileInfo, progress process.Progress, _ map[string]*domain.Details, allErrors *common.ConvertError) {
	progress.SetProgressMessage("Start creating link blocks")
	for _, file := range files {
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
	pathsCount int,
	files map[string]*FileInfo,
	progress process.Progress,
	details map[string]*domain.Details,
	allErrors *common.ConvertError,
) []*common.Snapshot {
	snapshots := make([]*common.Snapshot, 0)
	relationsSnapshots := make([]*common.Snapshot, 0)
	objectTypeSnapshots := make([]*common.Snapshot, 0)
	progress.SetProgressMessage("Start creating snapshots")

	// Check if we have schemas loaded
	hasSchemas := len(m.schemas) > 0

	// First pass: collect all YAML properties to create relation snapshots
	yamlRelations := make(map[string]*yamlProperty) // property name -> property
	objectTypes := make(map[string][]string)        // Track unique object type names

	for _, file := range files {
		var props = make([]string, 0, len(file.YAMLProperties))
		if file.YAMLProperties != nil {
			for i := range file.YAMLProperties {
				prop := &file.YAMLProperties[i]

				// If we have schemas, try to find the relation by name
				if hasSchemas {
					if schemaKey := m.getRelationKeyByName(prop.name); schemaKey != "" {
						// Use the schema-defined key
						prop.key = schemaKey
					}
				}

				// Use existing relation if already seen
				if _, exists := yamlRelations[prop.name]; !exists {
					yamlRelations[prop.name] = prop
				}
				props = append(props, yamlRelations[prop.name].key)
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
		relationsSnapshots = append(relationsSnapshots, m.createRelationSnapshots()...)

		// Create relation option snapshots from schemas
		relationsSnapshots = append(relationsSnapshots, m.createRelationOptionSnapshots()...)

		// Create type snapshots from schemas
		objectTypeSnapshots = append(objectTypeSnapshots, m.createTypeSnapshots()...)

		// Map type names to keys for later use
		objectTypeKeys = make(map[string]string)
		for typeName := range objectTypes {
			if typeKey := m.getTypeKeyByName(typeName); typeKey != "" {
				objectTypeKeys[typeName] = typeKey
			}
		}
	} else {
		// Fallback to original YAML-based creation
		// Create relation snapshots for YAML properties
		for propName, prop := range yamlRelations {
			// Generate BSON ID for the relation key
			relationDetails := getRelationDetails(propName, prop.key, float64(prop.format), prop.includeTime)

			relationsSnapshots = append(relationsSnapshots, &common.Snapshot{
				Id: propIdPrefix + prop.key,
				Snapshot: &common.SnapshotModel{
					SbType: smartblock.SmartBlockTypeRelation,
					Data: &common.StateSnapshot{
						Details:       relationDetails,
						RelationLinks: bundledRelationLinks(relationDetails),
						ObjectTypes:   []string{bundle.TypeKeyRelation.String()},
						Key:           prop.key,
					},
				},
			})
		}

		typeRelation := bundle.MustGetRelation(bundle.RelationKeyType)
		relationsSnapshots = append(relationsSnapshots, &common.Snapshot{
			Id: propIdPrefix + bundle.TypeKeyObjectType.String(),
			Snapshot: &common.SnapshotModel{
				SbType: smartblock.SmartBlockTypeRelation,
				Data: &common.StateSnapshot{
					Details:     getRelationDetails(typeRelation.Name, typeRelation.Key, float64(typeRelation.Format), false),
					ObjectTypes: []string{bundle.TypeKeyRelation.String()},
					Key:         bundle.TypeKeyObjectType.String(),
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
	}

	// Second pass: create object snapshots
	for name, file := range files {
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
					Key:    prop.key,
					Format: prop.format,
				})
			}
		}

		// Determine object type
		objectTypeKey := bundle.TypeKeyPage.String()
		if file.ObjectTypeName != "" {
			if typeKey, exists := objectTypeKeys[file.ObjectTypeName]; exists {
				objectTypeKey = typeKey
			}
		}

		snapshots = append(snapshots, &common.Snapshot{
			Id:       file.PageID,
			FileName: name,
			Snapshot: &common.SnapshotModel{
				SbType: smartblock.SmartBlockTypePage,
				Data: &common.StateSnapshot{
					Blocks:        file.ParsedBlocks,
					Details:       details[name],
					ObjectTypes:   []string{objectTypeKey},
					RelationLinks: relationLinks,
				}},
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

func (m *Markdown) addChildBlocks(files map[string]*FileInfo, progress process.Progress, _ map[string]*domain.Details, allErrors *common.ConvertError) {
	progress.SetProgressMessage("Start creating root blocks")
	childBlocks := m.extractChildBlocks(files)
	for _, file := range files {
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

func (m *Markdown) extractChildBlocks(files map[string]*FileInfo) map[string]struct{} {
	childBlocks := make(map[string]struct{})
	for _, file := range files {
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

func (m *Markdown) addLinkToObjectBlocks(files map[string]*FileInfo, progress process.Progress, _ map[string]*domain.Details, allErrors *common.ConvertError) {
	progress.SetProgressMessage("Start linking blocks")
	for _, file := range files {
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
				target, err := url.PathUnescape(link.TargetBlockId)
				if err != nil {
					log.Warnf("error while url.PathUnescape: %s", err)
					target = link.TargetBlockId
				}

				if files[target] != nil {
					link.TargetBlockId = files[target].PageID
					files[target].HasInboundLinks = true
				}

				continue
			}

			if text := block.GetText(); text != nil && text.Marks != nil && len(text.Marks.Marks) > 0 {
				for _, mark := range text.Marks.Marks {
					if mark.Type != model.BlockContentTextMark_Mention && mark.Type != model.BlockContentTextMark_Object {
						continue
					}

					if targetFile, exists := files[mark.Param]; exists {
						mark.Param = targetFile.PageID
					}
				}
			}
		}
	}
}

func (m *Markdown) fillEmptyBlocks(files map[string]*FileInfo, progress process.Progress, _ map[string]*domain.Details, allErrors *common.ConvertError) {
	progress.SetProgressMessage("Start creating file blocks")
	// process file blocks
	for _, file := range files {
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

func (m *Markdown) setNewID(files map[string]*FileInfo, progress process.Progress, details map[string]*domain.Details, allErrors *common.ConvertError) {
	progress.SetProgressMessage("Start creating blocks")
	for name, file := range files {
		if err := progress.TryStep(1); err != nil {
			allErrors.Add(common.ErrCancel)
			return
		}

		if strings.EqualFold(filepath.Ext(name), ".md") || strings.EqualFold(filepath.Ext(name), ".csv") {
			file.PageID = bson.NewObjectId().Hex()

			m.setDetails(file, name, details)
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

func (m *Markdown) retrieveRootObjectsIds(files map[string]*FileInfo) []string {
	var rootObjectsIds []string
	for _, file := range files {
		if file.PageID == "" {
			continue
		}
		if file.IsRootFile {
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

// loadSchemas loads all JSON schemas from the schemas folder using the schema package
func (m *Markdown) loadSchemas(importSource source.Source, allErrors *common.ConvertError) error {
	schemasFound := false
	parser := schema.NewJSONSchemaParser()

	// Iterate through files looking for schemas folder
	err := importSource.Iterate(func(fileName string, fileReader io.ReadCloser) bool {
		defer fileReader.Close()

		// Check if file is a JSON schema file
		if strings.HasSuffix(fileName, ".schema.json") {
			schemasFound = true

			// Parse schema using the schema package
			s, err := parser.Parse(fileReader)
			if err != nil {
				allErrors.Add(fmt.Errorf("failed to parse schema %s: %w", fileName, err))
				return true
			}

			// Get type name from schema
			typeName := ""
			if s.GetType() != nil {
				typeName = s.GetType().Name
			}

			if typeName == "" {
				// Try to derive from filename
				base := filepath.Base(fileName)
				typeName = strings.TrimSuffix(base, ".schema.json")
				typeName = strings.ReplaceAll(typeName, "_", " ")
				typeName = strings.Title(typeName)
			}

			if typeName != "" {
				m.schemas[typeName] = s
				log.Infof("Loaded schema for type: %s", typeName)
			}
		}

		return true
	})

	if err != nil {
		return err
	}

	if !schemasFound {
		log.Infof("No schemas folder found")
		return nil
	}

	log.Infof("Loaded %d schemas", len(m.schemas))
	return nil
}

// getRelationKeyByName returns the relation key for a given property name
func (m *Markdown) getRelationKeyByName(propName string) string {
	for _, s := range m.schemas {
		for relKey, rel := range s.Relations {
			if rel.Name == propName {
				return relKey
			}
		}
	}
	return ""
}

// getTypeKeyByName returns the type key for a given type name
func (m *Markdown) getTypeKeyByName(typeName string) string {
	if s, ok := m.schemas[typeName]; ok {
		if s.GetType() != nil {
			return s.GetType().Key
		}
	}
	return ""
}

// createRelationSnapshots creates snapshots for all discovered relations
func (m *Markdown) createRelationSnapshots() []*common.Snapshot {
	var snapshots []*common.Snapshot
	processedKeys := make(map[string]bool)

	for _, s := range m.schemas {
		for key, rel := range s.Relations {
			// Skip if already processed or if it's a bundled relation
			if processedKeys[key] {
				continue
			}
			if _, err := bundle.GetRelation(domain.RelationKey(key)); err == nil {
				continue
			}

			details := rel.ToDetails()

			snapshot := &common.Snapshot{
				Id: propIdPrefix + key,
				Snapshot: &common.SnapshotModel{
					SbType: smartblock.SmartBlockTypeRelation,
					Data: &common.StateSnapshot{
						Details:       details,
						RelationLinks: bundledRelationLinks(details),
						ObjectTypes:   []string{bundle.TypeKeyRelation.String()},
						Key:           key,
					},
				},
			}

			snapshots = append(snapshots, snapshot)
			processedKeys[key] = true
		}
	}

	return snapshots
}

// createRelationOptionSnapshots creates snapshots for relation options (status and tag examples)
func (m *Markdown) createRelationOptionSnapshots() []*common.Snapshot {
	var snapshots []*common.Snapshot

	for _, s := range m.schemas {
		for key, rel := range s.Relations {
			// Skip if this is a bundled relation
			if _, err := bundle.GetRelation(domain.RelationKey(key)); err == nil {
				continue
			}

			// Create options for status relations
			if rel.Format == model.RelationFormat_status && len(rel.Options) > 0 {
				for _, option := range rel.Options {
					details := rel.CreateOptionDetails(option, "")
					optionKey := bson.NewObjectId().Hex()

					snapshot := &common.Snapshot{
						Id: "opt_" + optionKey,
						Snapshot: &common.SnapshotModel{
							SbType: smartblock.SmartBlockTypeRelationOption,
							Data: &common.StateSnapshot{
								Details:       details,
								RelationLinks: bundledRelationLinks(details),
								ObjectTypes:   []string{bundle.TypeKeyRelationOption.String()},
								Key:           optionKey,
							},
						},
					}

					snapshots = append(snapshots, snapshot)
				}
			}

			// Create options for tag relations with examples
			if rel.Format == model.RelationFormat_tag && len(rel.Examples) > 0 {
				for _, example := range rel.Examples {
					details := rel.CreateOptionDetails(example, "")
					optionKey := bson.NewObjectId().Hex()

					snapshot := &common.Snapshot{
						Id: "opt_" + optionKey,
						Snapshot: &common.SnapshotModel{
							SbType: smartblock.SmartBlockTypeRelationOption,
							Data: &common.StateSnapshot{
								Details:       details,
								RelationLinks: bundledRelationLinks(details),
								ObjectTypes:   []string{bundle.TypeKeyRelationOption.String()},
								Key:           optionKey,
							},
						},
					}

					snapshots = append(snapshots, snapshot)
				}
			}
		}
	}

	return snapshots
}

// createTypeSnapshots creates snapshots for all discovered types
func (m *Markdown) createTypeSnapshots() []*common.Snapshot {
	var snapshots []*common.Snapshot

	for _, s := range m.schemas {
		if s.GetType() == nil {
			continue
		}

		typ := s.GetType()
		typeKey := typ.Key
		if typeKey == "" {
			typeKey = bson.NewObjectId().Hex()
		}

		typ.KeyToIdFunc = func(key string) string {
			return propIdPrefix + key
		}
		details := typ.ToDetails()

		snapshot := &common.Snapshot{
			Id: typeIdPrefix + typeKey,
			Snapshot: &common.SnapshotModel{
				SbType: smartblock.SmartBlockTypeObjectType,
				Data: &common.StateSnapshot{
					Details:       details,
					RelationLinks: bundledRelationLinks(details),
					ObjectTypes:   []string{bundle.TypeKeyObjectType.String()},
					Key:           typeKey,
				},
			},
		}

		snapshots = append(snapshots, snapshot)
	}

	return snapshots
}
