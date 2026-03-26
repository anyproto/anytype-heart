package service

import "sort"

const (
	rrfK            = 60
	rrfVectorWeight = 0.7
	rrfFtsWeight    = 0.3
)

type rankedObject struct {
	objectId string
	rrfScore float64
	ftsRank  int // 0 means not present in FTS results
	vecRank  int // 0 means not present in vector results
}

// rrfRerank combines FTS and vector result orderings using Reciprocal Rank Fusion.
// ftsIds and vecIds are ordered by their respective relevance (best first).
// Returns object IDs sorted by combined RRF score (best first).
func rrfRerank(ftsIds []string, vecIds []string) []string {
	ftsRanks := make(map[string]int, len(ftsIds))
	for i, id := range ftsIds {
		ftsRanks[id] = i + 1
	}

	vecRanks := make(map[string]int, len(vecIds))
	for i, id := range vecIds {
		vecRanks[id] = i + 1
	}

	// Collect union of all object IDs
	seen := make(map[string]struct{})
	var objects []rankedObject

	for _, id := range ftsIds {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		objects = append(objects, rankedObject{objectId: id, ftsRank: ftsRanks[id], vecRank: vecRanks[id]})
	}
	for _, id := range vecIds {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		objects = append(objects, rankedObject{objectId: id, ftsRank: ftsRanks[id], vecRank: vecRanks[id]})
	}

	// Compute RRF scores
	for i := range objects {
		score := 0.0
		if objects[i].vecRank > 0 {
			score += rrfVectorWeight / float64(rrfK+objects[i].vecRank)
		}
		if objects[i].ftsRank > 0 {
			score += rrfFtsWeight / float64(rrfK+objects[i].ftsRank)
		}
		objects[i].rrfScore = score
	}

	sort.Slice(objects, func(i, j int) bool {
		return objects[i].rrfScore > objects[j].rrfScore
	})

	result := make([]string, len(objects))
	for i, obj := range objects {
		result[i] = obj.objectId
	}
	return result
}
