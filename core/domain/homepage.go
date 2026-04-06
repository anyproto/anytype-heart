package domain

const (
	HomepageWidgets    = "widgets" // default
	HomepageGraph      = "graph"
	HomepageLastOpened = "lastOpened" // deprecated
	HomepageChat       = "chat"       // used for one-to-one channels
)

func IsHomepageConstant(homepage string) bool {
	return homepage == HomepageWidgets || homepage == HomepageLastOpened || homepage == HomepageGraph || homepage == HomepageChat
}
