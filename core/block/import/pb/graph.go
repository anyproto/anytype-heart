package pb

import (
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/import/converter"
)

// findBidirectionalLinks tries to find in LinksGraph such pairs object1 -> object2, object2 -> object1 and exclude them from Graph
func findBidirectionalLinks(graph converter.LinksGraph) (map[string][]string, converter.LinksGraph) {
	bidirectionalLinksPairs := make(map[string][]string, 0)
	graphWithoutBidirectionalLinks := make(converter.LinksGraph, 0)
	for objectID, links := range graph {
		var foundBacklink bool
		for link := range links {
			outboundLinks := graph[link]
			if _, ok := outboundLinks[objectID]; ok {
				addBidirectionalPair(bidirectionalLinksPairs, link, objectID)
				foundBacklink = true
			}
		}
		if !foundBacklink {
			graphWithoutBidirectionalLinks[objectID] = links
		}
	}
	return bidirectionalLinksPairs, graphWithoutBidirectionalLinks
}

func addBidirectionalPair(bidirectionalLinksPairs map[string][]string, link string, objectID string) {
	if _, ok := bidirectionalLinksPairs[link]; !ok || !lo.Contains(bidirectionalLinksPairs[link], objectID) {
		bidirectionalLinksPairs[link] = append(bidirectionalLinksPairs[link], objectID)
	}
	if _, ok := bidirectionalLinksPairs[objectID]; !ok || !lo.Contains(bidirectionalLinksPairs[objectID], link) {
		bidirectionalLinksPairs[objectID] = append(bidirectionalLinksPairs[objectID], link)
	}
}

// findBidirectionalLinksWithoutInboundLinks tries to find pairs like object1 -> object2, object2 -> object1
// with condition, that object1 and object2 doesn't have other inbound links. We assume, that these objects can be added to root collection
func findBidirectionalLinksWithoutInboundLinks(graphWithoutBidirectionalLinks converter.LinksGraph, bidirectionalLinks map[string][]string) []string {
	excludedLinks := make(map[string]bool, 0)
	var bidirectionalLinksWithoutInboundLinks []string
	visited := make(map[string]bool, 0)
	for _, objectLinks := range graphWithoutBidirectionalLinks {
		for objectID := range objectLinks {
			excludedLinks[objectID] = true
		}
	}
	for objectID := range bidirectionalLinks {
		if !visited[objectID] {
			bfs(bidirectionalLinks, objectID, visited, excludedLinks)
		}
	}
	excludedLinksSlice := lo.MapToSlice(excludedLinks, func(key string, value bool) string { return key })
	for objectID := range bidirectionalLinks {
		if !lo.Contains(excludedLinksSlice, objectID) {
			bidirectionalLinksWithoutInboundLinks = append(bidirectionalLinksWithoutInboundLinks, objectID)
		}
	}
	return bidirectionalLinksWithoutInboundLinks
}

func bfs(graph map[string][]string, startNode string, visited map[string]bool, excludedLinks map[string]bool) {
	queue := []string{startNode}

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]

		if visited[node] {
			continue
		}
		visited[node] = true
		for _, neighborNode := range graph[node] {
			if excludedLinks[node] {
				excludedLinks[neighborNode] = true
			}
			if excludedLinks[neighborNode] {
				excludedLinks[node] = true
			}
			if !visited[neighborNode] {
				queue = append(queue, neighborNode)
			}
		}
	}
}

func findObjectsWithoutInboundLinks(objectsLinks converter.LinksGraph, objects []string) []string {
	objectInLink := make(map[string]struct{}, 0)
	var rootObjects []string
	for _, links := range objectsLinks {
		for link := range links {
			if _, ok := objectInLink[link]; !ok {
				objectInLink[link] = struct{}{}
			}
		}
	}
	for _, object := range objects {
		if _, ok := objectInLink[object]; !ok {
			rootObjects = append(rootObjects, object)
		}
	}
	return rootObjects
}
