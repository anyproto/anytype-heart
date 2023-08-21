package pb

import (
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/import/converter"
)

// findBackLinks tries to find in objects Graph such pairs object1 -> object2, object2 -> object1 and exclude them from graph
func findBackLinks(objectsLinks converter.Graph) (map[string][]string, converter.Graph) {
	backlinksPairs := make(map[string][]string, 0)
	graphWithoutBacklinks := make(converter.Graph, 0)
	for objectID, links := range objectsLinks {
		var foundBacklink bool
		for link := range links {
			outboundLinks := objectsLinks[link]
			if _, ok := outboundLinks[objectID]; ok {
				backlinksPairs[link] = append(backlinksPairs[link], objectID)
				backlinksPairs[objectID] = append(backlinksPairs[objectID], link)
				foundBacklink = true
			}
		}
		if !foundBacklink {
			graphWithoutBacklinks[objectID] = links
		}
	}
	return backlinksPairs, graphWithoutBacklinks
}

// findBacklinksWithoutInboundLinks tries to find pairs like object1 -> object2, object2 -> object1
// with condition, that object1 and object2 doesn't have other inbound links
func findBacklinksWithoutInboundLinks(graphWithoutBacklinks converter.Graph, backlinks map[string][]string) []string {
	excludedBackLinks := make(map[string]bool, 0)
	var rootObjects []string
	visited := make(map[string]bool, 0)
	for _, objectLinks := range graphWithoutBacklinks {
		for objectID := range objectLinks {
			excludedBackLinks[objectID] = true
		}
	}

	for objectID := range backlinks {
		if !visited[objectID] {
			bfs(backlinks, objectID, visited, excludedBackLinks)
		}
	}
	excludedBackLinksSlice := lo.MapToSlice(excludedBackLinks, func(key string, value bool) string { return key })
	for objectID := range backlinks {
		if !lo.Contains(excludedBackLinksSlice, objectID) {
			rootObjects = append(rootObjects, objectID)
		}
	}
	return rootObjects
}

func bfs(graph map[string][]string, startNode string, visited map[string]bool, excludedBackLinks map[string]bool) {
	queue := []string{startNode}

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]

		if visited[node] {
			continue
		}
		visited[node] = true
		if excludedBackLinks[node] {
			for _, neighborNode := range graph[node] {
				excludedBackLinks[neighborNode] = true
				if !visited[neighborNode] {
					queue = append(queue, neighborNode)
				}
			}
		} else {
			for _, neighborNode := range graph[node] {
				if excludedBackLinks[node] {
					excludedBackLinks[neighborNode] = true
				}
				if excludedBackLinks[neighborNode] {
					excludedBackLinks[node] = true
				}
				if !visited[neighborNode] {
					queue = append(queue, neighborNode)
				}
			}
		}
	}
}

func findObjectsInLinks(objectsLinks converter.Graph) map[string]struct{} {
	objectInLink := make(map[string]struct{}, 0)
	for _, links := range objectsLinks {
		for link := range links {
			if _, ok := objectInLink[link]; !ok {
				objectInLink[link] = struct{}{}
			}
		}
	}
	return objectInLink
}
