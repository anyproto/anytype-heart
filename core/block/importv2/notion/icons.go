package notion

import (
	"strings"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

// Notion's built-in icon picker (API 2025-09-03) reports a NAMED icon:
//
//	"icon": {"type": "icon", "icon": {"name": "rocket", "color": "blue"}}
//
// which is the same kind of thing an Anytype type carries — a name from a
// closed vocabulary plus a color — so the two map straight across. The
// vocabularies differ: Notion's names are its own, Anytype's are the 390
// core/api/model.IconName constants the client ships a `type/<name>` sprite
// for. notionIconNames bridges them.
//
// Getting this wrong is not cosmetic. A database whose icon we cannot
// represent used to lose the planner's icon as well and render as the default
// glyph — in one real workspace 293 of 303 icons were named ones, so nearly
// every imported type came out blank.
//
// Notion publishes no enumeration of its icon names (the API reference gives
// examples only), so the table was inventoried against Notion's own icon
// files, which are public and named after the icon:
//
//	https://www.notion.so/icons/<name>_<color>.svg
//
// Every Anytype icon name was probed there (121 exist in both vocabularies and
// map to themselves), then candidate spellings and common icon words, then
// every name a real workspace's export used. A name Notion adds later is not
// lost: resolveNotionIconName still normalizes and shortens it, and failing
// that the planner's icon stands.
var notionIconNames = map[string]string{
	"accessibility":      "accessibility",
	"activity":           "pulse",
	"add":                "add-circle",
	"airplane":           "airplane",
	"alarm":              "alarm",
	"alert":              "alert-circle",
	"alien":              "planet",
	"ambulance":          "medkit",
	"anchor":             "boat",
	"apple":              "nutrition",
	"archive":            "archive",
	"arrow-down":         "arrow-down-circle",
	"arrow-left":         "arrow-back-circle",
	"arrow-right":        "arrow-forward-circle",
	"arrow-up":           "arrow-up-circle",
	"art":                "color-palette",
	"asterisk":           "star",
	"attachment":         "attach",
	"baby":               "person",
	"backpack":           "bag",
	"badge":              "ribbon",
	"bag":                "bag",
	"balloon":            "balloon",
	"barcode":            "barcode",
	"baseball":           "baseball",
	"basketball":         "basketball",
	"battery":            "battery-full",
	"battery-charging":   "battery-charging",
	"bed":                "bed",
	"bee":                "bug",
	"beer":               "beer",
	"bell":               "notifications",
	"bicycle":            "bicycle",
	"binoculars":         "binoculars",
	"bluetooth":          "bluetooth",
	"boat":               "boat",
	"bomb":               "nuclear",
	"bone":               "paw",
	"book":               "book",
	"book-closed":        "book",
	"bookmark":           "bookmark",
	"boot":               "footsteps",
	"bowl":               "restaurant",
	"bowling":            "bowling-ball",
	"brain":              "bulb",
	"branch":             "git-branch",
	"bread":              "nutrition",
	"briefcase":          "briefcase",
	"broom":              "brush",
	"bug":                "bug",
	"bullseye":           "locate",
	"burger":             "fast-food",
	"bus":                "bus",
	"butterfly":          "flower",
	"button":             "radio-button-on",
	"cafe":               "cafe",
	"cake":               "nutrition",
	"calculator":         "calculator",
	"calendar":           "calendar",
	"calendar-month":     "calendar",
	"camera":             "camera",
	"candy":              "nutrition",
	"car":                "car",
	"card":               "card",
	"cards":              "dice",
	"carrot":             "nutrition",
	"cash":               "cash",
	"cassette":           "disc",
	"castle":             "home",
	"cat":                "paw",
	"cellular":           "cellular",
	"chair":              "bed",
	"champagne":          "wine",
	"chart":              "bar-chart",
	"chat":               "chatbubble",
	"check":              "checkmark-circle",
	"checklist":          "list",
	"checkmark":          "checkmark-circle",
	"checkmark-line":     "checkmark-circle",
	"checkmark-square":   "checkbox",
	"chemistry":          "flask",
	"chicken":            "nutrition",
	"child":              "person",
	"church":             "business",
	"cigarette":          "flame",
	"circle":             "ellipse",
	"city":               "business",
	"clipping":           "cut",
	"clock":              "time",
	"clock-alternate":    "time",
	"close":              "close-circle",
	"cloud":              "cloud",
	"cloudy":             "cloudy",
	"clover":             "leaf",
	"coat":               "shirt",
	"code":               "code",
	"coffee":             "cafe",
	"color-palette":      "color-palette",
	"column":             "grid",
	"command-line":       "terminal",
	"comment":            "chatbubble",
	"compass":            "compass",
	"compose":            "create",
	"computer":           "desktop",
	"construction-crane": "construct",
	"contrast":           "contrast",
	"conversation":       "chatbubbles",
	"copy":               "copy",
	"corn":               "nutrition",
	"cow":                "paw",
	"create":             "create",
	"crop":               "crop",
	"currency":           "cash",
	"cursor":             "navigate",
	"customs":            "shield",
	"cut":                "cut",
	"dashboard":          "speedometer",
	"database":           "server",
	"delete":             "trash",
	"dental":             "medical",
	"dna":                "medical",
	"document":           "document",
	"dog":                "paw",
	"download":           "download",
	"drafts":             "file-tray",
	"dress":              "shirt",
	"drink":              "pint",
	"duck":               "paw",
	"ear":                "ear",
	"egg":                "egg",
	"elephant":           "paw",
	"emoji":              "happy",
	"error":              "close-circle",
	"exit":               "exit",
	"expand":             "expand",
	"extension":          "extension-puzzle",
	"factory":            "business",
	"feather":            "leaf",
	"feed":               "reader",
	"fire":               "flame",
	"fireworks":          "sparkles",
	"fish":               "fish",
	"flag":               "flag",
	"flash":              "flash",
	"flashlight":         "flashlight",
	"flatware":           "restaurant",
	"folder":             "folder",
	"football":           "football",
	"formula":            "calculator",
	"friends":            "people",
	"fuel":               "car",
	"gear":               "cog",
	"gem":                "diamond",
	"ghost":              "skull",
	"gift":               "gift",
	"git":                "git-branch",
	"glasses":            "glasses",
	"globe":              "globe",
	"golf":               "golf",
	"government":         "business",
	"grid":               "grid",
	"grocery":            "cart",
	"group":              "people",
	"groups":             "people",
	"guitar":             "musical-notes",
	"gym":                "barbell",
	"hammer":             "hammer",
	"hand":               "hand-right",
	"hanger":             "shirt",
	"hashtag":            "pricetag",
	"headphones":         "headset",
	"headset":            "headset",
	"heart":              "heart",
	"helicopter":         "airplane",
	"hexagon":            "shapes",
	"history":            "time",
	"home":               "home",
	"hourglass":          "hourglass",
	"inbox":              "file-tray-full",
	"infinity":           "infinite",
	"info":               "information-circle",
	"jar":                "beaker",
	"key":                "key",
	"keyboard":           "keypad",
	"keypad":             "keypad",
	"kite":               "balloon",
	"knife":              "restaurant",
	"language":           "language",
	"laptop":             "laptop",
	"layers":             "layers",
	"leaf":               "leaf",
	"lemon":              "nutrition",
	"library":            "library",
	"link":               "link",
	"list":               "list",
	"litter-disposal":    "trash-bin",
	"location":           "location",
	"lock":               "lock-closed",
	"log-in":             "log-in",
	"log-out":            "log-out",
	"luggage":            "bag-handle",
	"magnet":             "magnet",
	"mail":               "mail",
	"map":                "map",
	"map-pin":            "location",
	"map-pin-alternate":  "location",
	"meat":               "nutrition",
	"meeting":            "people",
	"megaphone":          "megaphone",
	"merge":              "git-merge",
	"microphone":         "mic",
	"microscope":         "flask",
	"mirror":             "aperture",
	"mobile":             "phone-portrait",
	"moon":               "moon",
	"mop":                "brush",
	"more":               "ellipsis-horizontal-circle",
	"motorcycle":         "bicycle",
	"mouth":              "happy",
	"move":               "move",
	"movie":              "film",
	"mushroom":           "nutrition",
	"music":              "musical-notes",
	"navigation":         "navigate",
	"network":            "git-network",
	"news":               "newspaper",
	"notification":       "notifications",
	"nut":                "nutrition",
	"onion":              "nutrition",
	"orange":             "nutrition",
	"orbit":              "planet",
	"oven":               "restaurant",
	"package":            "cube",
	"parking":            "car",
	"passport":           "id-card",
	"paste":              "clipboard",
	"pen":                "pencil",
	"pencil":             "pencil",
	"pentagon":           "shapes",
	"people":             "people",
	"perfume":            "rose",
	"phone":              "call",
	"piano":              "musical-notes",
	"pig":                "paw",
	"pill":               "medical",
	"pin":                "pin",
	"pizza":              "pizza",
	"playlist":           "musical-notes",
	"plus":               "add-circle",
	"power":              "power",
	"print":              "print",
	"profile":            "person-circle",
	"public":             "earth",
	"pull-request":       "git-pull-request",
	"pump":               "water",
	"puzzle":             "extension-puzzle",
	"radio":              "radio",
	"rain":               "rainy",
	"rainbow":            "color-palette",
	"receipt":            "receipt",
	"redo":               "arrow-redo",
	"refresh":            "refresh",
	"refrigerator":       "snow",
	"remove":             "remove-circle",
	"rename":             "create",
	"repeat":             "repeat",
	"reply":              "arrow-undo",
	"report":             "document-text",
	"robot":              "hardware-chip",
	"rocket":             "rocket",
	"row":                "list",
	"ruler":              "resize",
	"run":                "walk",
	"sandwich":           "fast-food",
	"save":               "save",
	"school":             "school",
	"science":            "flask",
	"script":             "code-slash",
	"search":             "search",
	"seed":               "leaf",
	"send":               "send",
	"server":             "server",
	"share":              "share",
	"shield":             "shield",
	"shirt":              "shirt",
	"shoe":               "footsteps",
	"shop":               "storefront",
	"shower":             "water",
	"shuffle":            "shuffle",
	"sink":               "water",
	"skateboard":         "bicycle",
	"skull":              "skull",
	"sliders-horizontal": "options",
	"slideshow":          "easel",
	"snake":              "paw",
	"snippet":            "document-text",
	"soap":               "water",
	"soccer":             "football",
	"sock":               "shirt",
	"spider":             "bug",
	"spoon":              "restaurant",
	"square":             "square",
	"stairs":             "footsteps",
	"star":               "star",
	"star-half":          "star-half",
	"stethoscope":        "medical",
	"sticker":            "pricetag",
	"stopwatch":          "stopwatch",
	"storm":              "thunderstorm",
	"strawberry":         "nutrition",
	"suit":               "shirt",
	"suitcase":           "briefcase",
	"sun":                "sunny",
	"sunglasses":         "glasses",
	"sunrise":            "partly-sunny",
	"sunset":             "partly-sunny",
	"sword":              "cut",
	"symbol":             "shapes",
	"sync":               "sync",
	"syringe":            "medical",
	"table":              "grid",
	"tablet":             "tablet-portrait",
	"tag":                "pricetag",
	"target":             "locate",
	"taxi":               "car",
	"teapot":             "cafe",
	"telephone":          "call",
	"telescope":          "telescope",
	"thought":            "chatbubble-ellipses",
	"thought-alert":      "chatbubble-ellipses",
	"thought-dialogue":   "chatbubbles",
	"thumbs-down":        "thumbs-down",
	"thumbs-up":          "thumbs-up",
	"ticket":             "ticket",
	"timeline":           "time",
	"toilet":             "water",
	"token":              "cash",
	"tooth":              "medical",
	"tornado":            "thunderstorm",
	"train":              "train",
	"translate":          "language",
	"tree":               "leaf",
	"triangle":           "triangle",
	"trophy":             "trophy",
	"truck":              "bus",
	"trumpet":            "musical-notes",
	"tshirt":             "shirt",
	"tv":                 "tv",
	"umbrella":           "umbrella",
	"undo":               "arrow-undo",
	"unlock":             "lock-open",
	"upload":             "cloud-upload",
	"user":               "person",
	"user-circle":        "person-circle",
	"verified":           "shield-checkmark",
	"view":               "eye",
	"violin":             "musical-notes",
	"volcano":            "flame",
	"volume-high":        "volume-high",
	"volume-off":         "volume-off",
	"walk":               "walk",
	"warning":            "warning",
	"water":              "water",
	"whale":              "fish",
	"wheat":              "leaf",
	"wheelchair":         "accessibility",
	"whistle":            "megaphone",
	"wifi":               "wifi",
	"wind":               "cloudy",
	"window":             "browsers",
	"wine":               "wine",
	"wrench":             "construct",
}

// notionIconVariants are Notion's variant suffixes ("clock-alternate" is the
// same clock, drawn differently). Stripping one and retrying covers variants
// of names already in the table without listing each one.
var notionIconVariants = []string{
	"-alternate", "-alternative", "-line", "-filled", "-outline",
	"-heavy", "-light", "-solid", "-square", "-circle",
}

// resolveNotionIconName maps one Notion icon name onto an Anytype icon.
//
// The table is the first answer and covers Notion's set as inventoried; the
// steps after it are for names added since. Notion documents its names as
// case-insensitive with spaces, underscores and hyphens equivalent, so that
// normalization comes first. Then variant suffixes, then progressively
// dropping trailing words — "chart-bar-stacked" is a chart, "book-open" is a
// book. Whatever is left has no counterpart, and the caller keeps the icon it
// already had.
func resolveNotionIconName(raw string) (string, bool) {
	name := strings.ToLower(strings.TrimSpace(raw))
	name = strings.Map(func(r rune) rune {
		if r == ' ' || r == '_' {
			return '-'
		}
		return r
	}, name)

	lookup := func(candidate string) (string, bool) {
		if resolved, ok := notionIconNames[candidate]; ok {
			return resolved, true
		}
		// Every suffix is tried, not just the first that matches: "-line" also
		// ends "book-outline", and giving up there would skip "-outline".
		for _, suffix := range notionIconVariants {
			stem := strings.TrimSuffix(candidate, suffix)
			if stem == candidate {
				continue
			}
			if resolved, ok := notionIconNames[stem]; ok {
				return resolved, true
			}
		}
		return "", false
	}

	for candidate := name; candidate != ""; {
		if resolved, ok := lookup(candidate); ok {
			return resolved, true
		}
		cut := strings.LastIndex(candidate, "-")
		if cut < 0 {
			break
		}
		candidate = candidate[:cut]
	}
	return "", false
}

// notionIconColors maps Notion's icon palette onto Anytype's ten icon options.
// Anytype has no plain green or brown, so those take the nearest neighbour.
// An unknown color must never cost the icon: grey is the fallback.
var notionIconColors = map[string]int64{
	"gray":       1,
	"grey":       1,
	"lightgray":  1,
	"light_gray": 1,
	"lightgrey":  1,
	"default":    1,
	"yellow":     2,
	"orange":     3,
	"brown":      3,
	"red":        4,
	"pink":       5,
	"purple":     6,
	"blue":       7,
	"green":      10,
}

const defaultIconOption int64 = 1

func notionIconOption(color string) int64 {
	if option, ok := notionIconColors[strings.ToLower(strings.TrimSpace(color))]; ok {
		return option
	}
	return defaultIconOption
}

// namedIcon resolves a Notion built-in icon to an Anytype icon name and color.
// Not-ok means "no named icon to apply": a different icon kind, or a name with
// no Anytype counterpart.
func (i *iconValue) namedIcon() (string, int64, bool) {
	if i == nil || i.Type != "icon" || i.Icon == nil || i.Icon.Name == "" {
		return "", 0, false
	}
	resolved, ok := resolveNotionIconName(i.Icon.Name)
	if !ok {
		return "", 0, false
	}
	return resolved, notionIconOption(i.Icon.Color), true
}

// applyDatabaseTypeIcon gives a database-backed type the database's own icon
// and returns whatever applyIcon must still materialize (nil when there is
// nothing left to do).
//
// The planner's icon is the fallback throughout: it is cleared only when
// something a type can actually show takes its place. The client renders a
// type's icon as image, then name, then emoji — so a planned name left in
// place would hide the database's own emoji, and clearing it for an icon we
// then fail to set leaves the type blank.
func applyDatabaseTypeIcon(details *domain.Details, icon *iconValue) *iconValue {
	if name, option, ok := icon.namedIcon(); ok {
		details.SetString(bundle.RelationKeyIconName, name)
		details.SetInt64(bundle.RelationKeyIconOption, option)
		return nil
	}
	if (icon.isEmoji() && icon.Emoji != "") || icon.fileUrl() != "" {
		details.Delete(bundle.RelationKeyIconName)
		details.Delete(bundle.RelationKeyIconOption)
		return icon
	}
	return nil
}
