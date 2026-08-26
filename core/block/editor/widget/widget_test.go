package widget

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The predefined-target inventory is import behaviour, not decoration:
// common.handleLinkBlock keeps a widget link target exactly when this
// function knows it, and rewrites anything else it cannot resolve to
// addr.MissingObject — after which WidgetObject.Init strips the link and its
// wrapper, losing the widget with no error. The function used to know four
// of the eight listings live spaces actually hold (allObjects comes from
// WidgetObject's own migration 3, chat and bin from the clients), so an app
// export re-imported without exactly those widgets.
func TestIsPredefinedWidgetTargetId(t *testing.T) {
	for _, id := range []string{
		DefaultWidgetFavorite, DefaultWidgetSet, DefaultWidgetRecentlyEdited, DefaultWidgetCollection,
		DefaultWidgetAll, DefaultWidgetRecentlyOpened, DefaultWidgetChat, DefaultWidgetBin,
	} {
		assert.True(t, IsPredefinedWidgetTargetId(id), id)
	}
	for _, id := range []string{"", "bafyreiamuhvd4f72swuxg6ejudiyfsinp56dkpr7crbnq3ulrdmvu7fryy", "page-home", "_favorite"} {
		assert.False(t, IsPredefinedWidgetTargetId(id), id)
	}
}
