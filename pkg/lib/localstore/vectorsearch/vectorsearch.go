package vectorsearch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/cheggaaa/mb/v3"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

const CName = "vectorsearch"

var log = logging.Logger("anytype-vectorsearch")

const minSimilarityScore float32 = 0.3

type ObjectScore struct {
	ObjectID   string
	Score      float32
	ChunkTitle string
	ChunkText  string
}

type VectorSearch interface {
	app.ComponentRunnable
	TryEnqueue(task SemanticTask)
	Search(ctx context.Context, spaceId string, query string, limit int) ([]ObjectScore, error)
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
	if cfg.EmbeddingApiKey == "" {
		log.Warn("vector search is enabled but TOGETHER_API_KEY is not set, disabling")
		return nil
	}

	v.qdrant = NewQdrantClient(cfg.QdrantAddr)
	v.embedder = NewEmbeddingClient(cfg.EmbeddingApiUrl, cfg.EmbeddingApiKey, cfg.EmbeddingModel)
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

func (v *vectorSearch) Search(ctx context.Context, spaceId string, query string, limit int) ([]ObjectScore, error) {
	if v.embedder == nil || v.qdrant == nil {
		return nil, nil // disabled
	}

	vectors, err := v.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(vectors) == 0 || len(vectors[0]) == 0 {
		return nil, nil
	}

	collection := collectionName(spaceId)
	results, err := v.qdrant.SearchPoints(ctx, collection, vectors[0], limit*3)
	if err != nil {
		return nil, fmt.Errorf("search qdrant: %w", err)
	}

	// Group by object_id, keep best score per object
	type bestMatch struct {
		score float32
		title string
		text  string
	}
	byObject := make(map[string]*bestMatch)

	for _, r := range results {
		if r.Score < minSimilarityScore {
			continue
		}
		objectID, _ := r.Payload["object_id"].(string)
		if objectID == "" {
			continue
		}
		title, _ := r.Payload["title"].(string)
		text, _ := r.Payload["text"].(string)

		if existing, ok := byObject[objectID]; !ok || r.Score > existing.score {
			byObject[objectID] = &bestMatch{score: r.Score, title: title, text: text}
		}
	}

	scores := make([]ObjectScore, 0, len(byObject))
	for objID, m := range byObject {
		scores = append(scores, ObjectScore{
			ObjectID:   objID,
			Score:      m.score,
			ChunkTitle: m.title,
			ChunkText:  m.text,
		})
	}

	// Sort by score descending
	for i := range scores {
		for j := i + 1; j < len(scores); j++ {
			if scores[j].Score > scores[i].Score {
				scores[i], scores[j] = scores[j], scores[i]
			}
		}
	}

	if len(scores) > limit {
		scores = scores[:limit]
	}

	return scores, nil
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

	collection := collectionName(task.SpaceID)

	if err := v.qdrant.EnsureCollection(ctx, collection, v.config.VectorSearch.EmbeddingDimensions); err != nil {
		return fmt.Errorf("ensure collection: %w", err)
	}

	// Fetch existing points for this object to reuse unchanged embeddings
	existingPoints, _ := v.qdrant.ScrollByObjectID(ctx, collection, task.ObjectID)
	cachedVectors := make(map[string][]float32) // hash -> vector
	for _, p := range existingPoints {
		if h, ok := p.Payload["hash"].(string); ok && len(p.Vector) > 0 {
			cachedVectors[h] = p.Vector
		}
	}

	// Compute hashes for new chunks and figure out which need embedding
	type chunkWithHash struct {
		chunk TextChunk
		text  string // title + text combined for embedding
		hash  string
	}
	items := make([]chunkWithHash, len(chunks))
	var textsToEmbed []string
	var embedIndexes []int // maps textsToEmbed index → items index

	for i, chunk := range chunks {
		t := chunk.Text
		if chunk.Title != "" {
			t = chunk.Title + "\n" + t
		}
		h := hashText(t)
		items[i] = chunkWithHash{chunk: chunk, text: t, hash: h}

		if _, cached := cachedVectors[h]; !cached {
			textsToEmbed = append(textsToEmbed, t)
			embedIndexes = append(embedIndexes, i)
		}
	}

	log.Warnf("[vectorsearch] object=%s chunks=%d cached=%d to_embed=%d",
		task.ObjectID, len(chunks), len(chunks)-len(textsToEmbed), len(textsToEmbed))

	// Embed only new/changed chunks
	var newVectors [][]float32
	if len(textsToEmbed) > 0 {
		var err error
		newVectors, err = v.embedder.Embed(ctx, textsToEmbed)
		if err != nil {
			return fmt.Errorf("embed chunks: %w", err)
		}
	}

	// Delete old points and build new ones
	if err := v.qdrant.DeletePointsByObjectID(ctx, collection, task.ObjectID); err != nil {
		return fmt.Errorf("delete old points: %w", err)
	}

	// Assign vectors: cached or newly embedded
	embIdx := 0
	points := make([]QdrantPoint, len(items))
	for i, item := range items {
		var vector []float32
		if cached, ok := cachedVectors[item.hash]; ok {
			vector = cached
		} else {
			vector = newVectors[embIdx]
			embIdx++
		}
		points[i] = QdrantPoint{
			ID:     item.chunk.ID,
			Vector: vector,
			Payload: map[string]any{
				"space_id":     item.chunk.SpaceID,
				"object_id":    item.chunk.ObjectID,
				"position":     item.chunk.Position,
				"title":        item.chunk.Title,
				"object_title": item.chunk.ObjectTitle,
				"text":         item.chunk.Text,
				"hash":         item.hash,
			},
		}
	}

	if err := v.qdrant.UpsertPoints(ctx, collection, points); err != nil {
		return fmt.Errorf("upsert points: %w", err)
	}

	log.Warnf("[vectorsearch] SUCCESS object=%s → %d points (%d reused, %d embedded) in collection=%s",
		task.ObjectID, len(points), len(points)-len(textsToEmbed), len(textsToEmbed), collection)
	return nil
}

func hashText(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:16]) // 128-bit hash is plenty for dedup
}

func collectionName(spaceID string) string {
	return "anytype_" + spaceID
}
