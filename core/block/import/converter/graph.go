package converter

type Neighbors map[string]struct{}

// LinksGraph contains objects with its outbound links
// Page1 -> Page2
// Page2 -> Page3, Page4
type LinksGraph map[string]Neighbors

func (g LinksGraph) AddNeighborToGraphNode(objectID string, newTarget string) {
	if _, ok := g[objectID]; !ok {
		g[objectID] = make(map[string]struct{}, 0)
	}
	if _, ok := g[objectID][newTarget]; !ok {
		g[objectID][newTarget] = struct{}{}
	}
}
