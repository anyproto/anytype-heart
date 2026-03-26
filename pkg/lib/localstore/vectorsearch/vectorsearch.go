package vectorsearch

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/cheggaaa/mb/v3"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

const CName = "vectorsearch"

var log = logging.Logger("anytype-vectorsearch")

type VectorSearch interface {
	app.ComponentRunnable
	TryEnqueue(task SemanticTask)
}

type vectorSearch struct {
	config   *config.Config
	queue    *mb.MB[SemanticTask]
	qdrant   QdrantClient
	embedder EmbeddingClient
	runCtx   context.Context
	cancel   context.CancelFunc
	done     chan struct{}
}

func New() VectorSearch {
	return &vectorSearch{}
}

func (v *vectorSearch) Init(a *app.App) (err error) {
	v.config = app.MustComponent[*config.Config](a)
	v.queue = mb.New[SemanticTask](1000)
	return nil
}

func (v *vectorSearch) Name() string {
	return CName
}

func (v *vectorSearch) Run(ctx context.Context) error {
	cfg := v.config.VectorSearch
	if !cfg.Enabled {
		log.Info("vector search is disabled")
		return nil
	}
	if cfg.OpenAIAPIKey == "" {
		log.Warn("vector search is enabled but OPENAI_API_KEY is not set, disabling")
		return nil
	}

	v.qdrant = NewQdrantClient(cfg.QdrantAddr)
	v.embedder = NewOpenAIEmbeddingClient(cfg.OpenAIAPIKey, cfg.EmbeddingModel, cfg.EmbeddingDimensions)
	v.runCtx, v.cancel = context.WithCancel(ctx)
	v.done = make(chan struct{})

	go v.processLoop()

	log.Info("vector search started")
	return nil
}

func (v *vectorSearch) Close(ctx context.Context) error {
	if v.cancel != nil {
		v.cancel()
	}
	if v.queue != nil {
		_ = v.queue.Close()
	}
	if v.done != nil {
		<-v.done
	}
	return nil
}

func (v *vectorSearch) TryEnqueue(task SemanticTask) {
	if v.done == nil {
		return // not running (disabled)
	}
	log.Warnf("[vectorsearch] enqueue object=%s space=%s title=%q blocks=%d",
		task.ObjectID, task.SpaceID, task.ObjectTitle, len(task.Blocks))
	_ = v.queue.TryAdd(task) //nolint:errcheck
}

func (v *vectorSearch) processLoop() {
	defer close(v.done)
	for {
		tasks, err := v.queue.Wait(v.runCtx)
		if err != nil {
			return // context cancelled or queue closed
		}
		for _, task := range tasks {
			if err := v.processTask(v.runCtx, task); err != nil {
				log.With("objectId", task.ObjectID, "spaceId", task.SpaceID).
					Errorf("process semantic task: %v", err)
			}
		}
	}
}

func (v *vectorSearch) processTask(ctx context.Context, task SemanticTask) error {
	chunks := ChunkBlocks(task)
	if len(chunks) == 0 {
		log.Warnf("[vectorsearch] object=%s produced 0 chunks, skipping", task.ObjectID)
		return nil
	}

	log.Warnf("[vectorsearch] object=%s chunked into %d chunks", task.ObjectID, len(chunks))
	for i, chunk := range chunks {
		log.Warnf("[vectorsearch]   chunk[%d] title=%q text_len=%d", i, chunk.Title, len(chunk.Text))
	}

	texts := make([]string, len(chunks))
	for i, chunk := range chunks {
		// Combine title and text for embedding
		t := chunk.Text
		if chunk.Title != "" {
			t = chunk.Title + "\n" + t
		}
		texts[i] = t
	}

	log.Warnf("[vectorsearch] calling OpenAI embeddings for %d texts...", len(texts))
	vectors, err := v.embedder.Embed(ctx, texts)
	if err != nil {
		return fmt.Errorf("embed chunks: %w", err)
	}
	log.Warnf("[vectorsearch] got %d embeddings (dim=%d)", len(vectors), len(vectors[0]))

	collection := collectionName(task.SpaceID)
	log.Warnf("[vectorsearch] upserting %d points to qdrant collection=%s", len(chunks), collection)

	if err := v.qdrant.EnsureCollection(ctx, collection, v.config.VectorSearch.EmbeddingDimensions); err != nil {
		return fmt.Errorf("ensure collection: %w", err)
	}

	// Remove old points for this object before upserting new ones
	if err := v.qdrant.DeletePointsByObjectID(ctx, collection, task.ObjectID); err != nil {
		return fmt.Errorf("delete old points: %w", err)
	}

	points := make([]QdrantPoint, len(chunks))
	for i, chunk := range chunks {
		points[i] = QdrantPoint{
			ID:     chunk.ID,
			Vector: vectors[i],
			Payload: map[string]any{
				"space_id":     chunk.SpaceID,
				"object_id":    chunk.ObjectID,
				"position":     chunk.Position,
				"title":        chunk.Title,
				"object_title": chunk.ObjectTitle,
				"text":         chunk.Text,
			},
		}
	}

	if err := v.qdrant.UpsertPoints(ctx, collection, points); err != nil {
		return fmt.Errorf("upsert points: %w", err)
	}

	log.Warnf("[vectorsearch] SUCCESS object=%s → %d points in collection=%s", task.ObjectID, len(points), collection)
	return nil
}

func collectionName(spaceID string) string {
	return "anytype_" + spaceID
}
