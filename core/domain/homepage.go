package domain

const (
	HomepageWidgets    = "widgets" // default
	HomepageGraph      = "graph"
	HomepageLastOpened = "lastOpened" // deprecated
)

func IsHomepageConstant(homepage string) bool {
	return homepage == HomepageWidgets || homepage == HomepageLastOpened || homepage == HomepageGraph
}
