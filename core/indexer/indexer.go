package indexer

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block"
	editorsb "github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

const (
	CName = "indexer"
)

var log = logging.Logger("anytype-doc-indexer")

func New() Indexer {
	return &indexer{
		indexedFiles: &sync.Map{},
	}
}

type Indexer interface {
	ForceFTIndex()
	StartFullTextIndex() error
	ReindexCommonObjects() error
	ReindexSpace(spaceID string) error
	Index(ctx context.Context, info editorsb.DocInfo, options ...editorsb.IndexOption) error
	app.ComponentRunnable
}

type Hasher interface {
	Hash() string
}

type spaceIDResolver interface {
	BindSpaceID(spaceID string, objectID string) error
	ResolveSpaceID(objectID string) (spaceID string, err error)
}

type spaceIDCache interface {
	BindSpaceID(spaceID string, objectID string) error
}

type objectCreator interface {
	CreateObject(ctx context.Context, spaceID string, req block.DetailsGetter, objectTypeKey domain.TypeKey) (id string, details *types.Struct, err error)
	InstallBundledObjects(
		ctx context.Context,
		spaceID string,
		sourceObjectIds []string,
	) (ids []string, objects []*types.Struct, err error)
}

type personalIDProvider interface {
	PersonalSpaceID() string
}

type indexer struct {
	store         objectstore.ObjectStore
	fileStore     filestore.FileStore
	resolver      spaceIDResolver
	source        source.Service
	picker        block.Picker
	ftsearch      ftsearch.FTSearch
	objectCreator objectCreator
	fileService   files.Service

	quit       chan struct{}
	btHash     Hasher
	newAccount bool
	forceFt    chan struct{}

	typeProvider typeprovider.SmartBlockTypeProvider
	spaceCore    spacecore.SpaceCoreService
	provider     personalIDProvider

	indexedFiles     *sync.Map
	reindexLogFields []zap.Field

	flags reindexFlags
}

func (i *indexer) Init(a *app.App) (err error) {
	i.newAccount = a.MustComponent(config.CName).(*config.Config).NewAccount
	i.store = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	i.resolver = app.MustComponent[spaceIDResolver](a)
	i.typeProvider = a.MustComponent(typeprovider.CName).(typeprovider.SmartBlockTypeProvider)
	i.source = a.MustComponent(source.CName).(source.Service)
	i.btHash = a.MustComponent("builtintemplate").(Hasher)
	i.fileStore = app.MustComponent[filestore.FileStore](a)
	i.ftsearch = app.MustComponent[ftsearch.FTSearch](a)
	i.objectCreator = app.MustComponent[objectCreator](a)
	i.picker = app.MustComponent[block.Picker](a)
	i.spaceCore = app.MustComponent[spacecore.SpaceCoreService](a)
	i.provider = app.MustComponent[personalIDProvider](a)
	i.fileService = app.MustComponent[files.Service](a)
	i.quit = make(chan struct{})
	i.forceFt = make(chan struct{})
	return
}

func (i *indexer) Name() (name string) {
	return CName
}

func (i *indexer) Run(context.Context) (err error) {
	return
}

func (i *indexer) StartFullTextIndex() (err error) {
	if ftErr := i.ftInit(); ftErr != nil {
		log.Errorf("can't init ft: %v", ftErr)
	}
	go i.ftLoop()
	return
}

func (i *indexer) Close(ctx context.Context) (err error) {
	close(i.quit)
	return nil
}

func (i *indexer) Index(ctx context.Context, info editorsb.DocInfo, options ...editorsb.IndexOption) error {
	// options are stored in smartblock pkg because of cyclic dependency :(
	startTime := time.Now()
	opts := &editorsb.IndexOptions{}
	for _, o := range options {
		o(opts)
	}
	sbType, err := i.typeProvider.Type(info.SpaceID, info.Id)
	if err != nil {
		sbType = smartblock.SmartBlockTypePage
	}
	if info.SpaceID == "" || pbtypes.GetString(info.State.CombinedDetails(), bundle.RelationKeySpaceId.String()) == "" {
		log.Warnf("index spaceID is empty for object %s %v", info.Id, info.State.ObjectTypeKeys())
	}
	headHashToIndex := headsHash(info.Heads)
	saveIndexedHash := func() {
		if headHashToIndex == "" {
			return
		}

		err = i.store.SaveLastIndexedHeadsHash(info.Id, headHashToIndex)
		if err != nil {
			log.With("objectID", info.Id).Errorf("failed to save indexed heads hash: %v", err)
		}
	}

	indexDetails, indexLinks := sbType.Indexable()
	if !indexDetails && !indexLinks {
		saveIndexedHash()
		return nil
	}

	lastIndexedHash, err := i.store.GetLastIndexedHeadsHash(info.Id)
	if err != nil {
		log.With("object", info.Id).Errorf("failed to get last indexed heads hash: %v", err)
	}

	if opts.SkipIfHeadsNotChanged {
		if headHashToIndex == "" {
			log.With("objectID", info.Id).Errorf("heads hash is empty")
		} else if lastIndexedHash == headHashToIndex {
			log.With("objectID", info.Id).Debugf("heads not changed, skipping indexing")

			// todo: the optimization temporarily disabled to see the metrics
			// return nil
		}
	}

	details := info.State.CombinedDetails()

	indexSetTime := time.Now()
	var hasError bool
	if indexLinks {
		if err = i.store.UpdateObjectLinks(info.Id, info.Links); err != nil {
			hasError = true
			log.With("objectID", info.Id).Errorf("failed to save object links: %v", err)
		}
	}

	indexLinksTime := time.Now()
	if indexDetails {
		if err := i.store.UpdateObjectDetails(info.Id, details); err != nil {
			if errors.Is(err, objectstore.ErrDetailsNotChanged) {
				metrics.ObjectDetailsHeadsNotChangedCounter.Add(1)
				log.With("objectID", info.Id).With("hashesAreEqual", lastIndexedHash == headHashToIndex).With("lastHashIsEmpty", lastIndexedHash == "").With("skipFlagSet", opts.SkipIfHeadsNotChanged).Debugf("details have not changed")
			} else {
				hasError = true
				log.With("objectID", info.Id).Errorf("can't update object store: %v", err)
			}
		} else {
			// todo: remove temp log
			if lastIndexedHash == headHashToIndex {
				l := log.With("objectID", info.Id).
					With("hashesAreEqual", lastIndexedHash == headHashToIndex).
					With("lastHashIsEmpty", lastIndexedHash == "").
					With("skipFlagSet", opts.SkipIfHeadsNotChanged)
				// With("old", pbtypes.Sprint(oldDetails.Details)).
				// With("new", pbtypes.Sprint(info.State.CombinedDetails())).
				// With("diff", pbtypes.Sprint(pbtypes.StructDiff(oldDetails.Details, info.State.CombinedDetails())))

				if opts.SkipIfHeadsNotChanged {
					l.Warnf("details have changed, but heads are equal")
				} else {
					l.Debugf("details have changed, but heads are equal")
				}
			}
		}

		// todo: the optimization temporarily disabled to see the metrics
		if true || !(opts.SkipFullTextIfHeadsNotChanged && lastIndexedHash == headHashToIndex) {
			if err := i.store.AddToIndexQueue(info.Id); err != nil {
				log.With("objectID", info.Id).Errorf("can't add id to index queue: %v", err)
			}
		}

		i.indexLinkedFiles(ctx, info.SpaceID, info.FileHashes)
	} else {
		_ = i.store.DeleteDetails(info.Id)
	}
	indexDetailsTime := time.Now()
	detailsCount := 0
	if details.GetFields() != nil {
		detailsCount = len(details.GetFields())
	}

	if !hasError {
		saveIndexedHash()
	}

	i.resolver.BindSpaceID(info.SpaceID, info.Id)

	metrics.SharedClient.RecordEvent(metrics.IndexEvent{
		ObjectId:                info.Id,
		IndexLinksTimeMs:        indexLinksTime.Sub(indexSetTime).Milliseconds(),
		IndexDetailsTimeMs:      indexDetailsTime.Sub(indexLinksTime).Milliseconds(),
		IndexSetRelationsTimeMs: indexSetTime.Sub(startTime).Milliseconds(),
		RelationsCount:          len(info.State.PickRelationLinks()),
		DetailsCount:            detailsCount,
	})

	return nil
}

func (i *indexer) indexLinkedFiles(ctx context.Context, spaceID string, fileHashes []string) {
	if len(fileHashes) == 0 {
		return
	}
	existingIDs, err := i.store.HasIDs(fileHashes...)
	if err != nil {
		log.Errorf("failed to get existing file ids : %s", err.Error())
	}
	newIDs := slice.Difference(fileHashes, existingIDs)
	for _, id := range newIDs {
		go func(id string) {
			// Deduplicate
			_, ok := i.indexedFiles.LoadOrStore(id, struct{}{})
			if ok {
				return
			}
			err := i.resolver.BindSpaceID(spaceID, id)
			if err != nil {
				log.Error("failed to bind space id", zap.Error(err), zap.String("id", id))
				return
			}
			// file's hash is id
			idxErr := i.reindexDoc(ctx, id)
			if idxErr != nil && !errors.Is(idxErr, domain.ErrFileNotFound) {
				log.With("id", id).Errorf("failed to reindex file: %s", idxErr)
			}
			idxErr = i.store.AddToIndexQueue(id)
			if idxErr != nil {
				log.With("id", id).Error(idxErr.Error())
			}
		}(id)
	}
}

func (i *indexer) getObjectInfo(ctx context.Context, id string) (info editorsb.DocInfo, err error) {
	err = block.DoContext(i.picker, ctx, id, func(sb editorsb.SmartBlock) error {
		info = sb.GetDocInfo()
		return nil
	})
	return
}

func headsHash(heads []string) string {
	if len(heads) == 0 {
		return ""
	}
	slices.Sort(heads)

	sum := sha256.Sum256([]byte(strings.Join(heads, ",")))
	return fmt.Sprintf("%x", sum)
}
