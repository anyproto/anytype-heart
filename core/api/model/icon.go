package apimodel

import (
	"encoding/json"
	"fmt"

	"github.com/anyproto/anytype-heart/core/api/util"
)

type IconFormat string

const (
	IconFormatEmoji IconFormat = "emoji"
	IconFormatFile  IconFormat = "file"
	IconFormatIcon  IconFormat = "icon"
)

func (f *IconFormat) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch IconFormat(s) {
	case IconFormatEmoji, IconFormatFile, IconFormatIcon:
		*f = IconFormat(s)
		return nil
	default:
		return util.ErrBadInput(fmt.Sprintf("invalid icon format: %q", s))
	}
}

type Color string

const (
	ColorGrey   Color = "grey"
	ColorYellow Color = "yellow"
	ColorOrange Color = "orange"
	ColorRed    Color = "red"
	ColorPink   Color = "pink"
	ColorPurple Color = "purple"
	ColorBlue   Color = "blue"
	ColorIce    Color = "ice"
	ColorTeal   Color = "teal"
	ColorLime   Color = "lime"
)

func (c *Color) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch Color(s) {
	case ColorGrey, ColorYellow, ColorOrange, ColorRed, ColorPink, ColorPurple, ColorBlue, ColorIce, ColorTeal, ColorLime:
		*c = Color(s)
		return nil
	default:
		return util.ErrBadInput(fmt.Sprintf("invalid color: %q", s))
	}
}

type IconName string

const (
	IconNameAccessibility            IconName = "accessibility"
	IconNameAddCircle                IconName = "add-circle"
	IconNameAirplane                 IconName = "airplane"
	IconNameAlarm                    IconName = "alarm"
	IconNameAlbums                   IconName = "albums"
	IconNameAlertCircle              IconName = "alert-circle"
	IconNameAmericanFootball         IconName = "american-football"
	IconNameAnalytics                IconName = "analytics"
	IconNameAperture                 IconName = "aperture"
	IconNameApps                     IconName = "apps"
	IconNameArchive                  IconName = "archive"
	IconNameArrowBackCircle          IconName = "arrow-back-circle"
	IconNameArrowDownCircle          IconName = "arrow-down-circle"
	IconNameArrowForwardCircle       IconName = "arrow-forward-circle"
	IconNameArrowRedoCircle          IconName = "arrow-redo-circle"
	IconNameArrowRedo                IconName = "arrow-redo"
	IconNameArrowUndoCircle          IconName = "arrow-undo-circle"
	IconNameArrowUndo                IconName = "arrow-undo"
	IconNameArrowUpCircle            IconName = "arrow-up-circle"
	IconNameAtCircle                 IconName = "at-circle"
	IconNameAttach                   IconName = "attach"
	IconNameBackspace                IconName = "backspace"
	IconNameBagAdd                   IconName = "bag-add"
	IconNameBagCheck                 IconName = "bag-check"
	IconNameBagHandle                IconName = "bag-handle"
	IconNameBagRemove                IconName = "bag-remove"
	IconNameBag                      IconName = "bag"
	IconNameBalloon                  IconName = "balloon"
	IconNameBan                      IconName = "ban"
	IconNameBandage                  IconName = "bandage"
	IconNameBarChart                 IconName = "bar-chart"
	IconNameBarbell                  IconName = "barbell"
	IconNameBarcode                  IconName = "barcode"
	IconNameBaseball                 IconName = "baseball"
	IconNameBasket                   IconName = "basket"
	IconNameBasketball               IconName = "basketball"
	IconNameBatteryCharging          IconName = "battery-charging"
	IconNameBatteryDead              IconName = "battery-dead"
	IconNameBatteryFull              IconName = "battery-full"
	IconNameBatteryHalf              IconName = "battery-half"
	IconNameBeaker                   IconName = "beaker"
	IconNameBed                      IconName = "bed"
	IconNameBeer                     IconName = "beer"
	IconNameBicycle                  IconName = "bicycle"
	IconNameBinoculars               IconName = "binoculars"
	IconNameBluetooth                IconName = "bluetooth"
	IconNameBoat                     IconName = "boat"
	IconNameBody                     IconName = "body"
	IconNameBonfire                  IconName = "bonfire"
	IconNameBook                     IconName = "book"
	IconNameBookmark                 IconName = "bookmark"
	IconNameBookmarks                IconName = "bookmarks"
	IconNameBowlingBall              IconName = "bowling-ball"
	IconNameBriefcase                IconName = "briefcase"
	IconNameBrowsers                 IconName = "browsers"
	IconNameBrush                    IconName = "brush"
	IconNameBug                      IconName = "bug"
	IconNameBuild                    IconName = "build"
	IconNameBulb                     IconName = "bulb"
	IconNameBus                      IconName = "bus"
	IconNameBusiness                 IconName = "business"
	IconNameCafe                     IconName = "cafe"
	IconNameCalculator               IconName = "calculator"
	IconNameCalendarClear            IconName = "calendar-clear"
	IconNameCalendarNumber           IconName = "calendar-number"
	IconNameCalendar                 IconName = "calendar"
	IconNameCall                     IconName = "call"
	IconNameCameraReverse            IconName = "camera-reverse"
	IconNameCamera                   IconName = "camera"
	IconNameCarSport                 IconName = "car-sport"
	IconNameCar                      IconName = "car"
	IconNameCard                     IconName = "card"
	IconNameCaretBackCircle          IconName = "caret-back-circle"
	IconNameCaretBack                IconName = "caret-back"
	IconNameCaretDownCircle          IconName = "caret-down-circle"
	IconNameCaretDown                IconName = "caret-down"
	IconNameCaretForwardCircle       IconName = "caret-forward-circle"
	IconNameCaretForward             IconName = "caret-forward"
	IconNameCaretUpCircle            IconName = "caret-up-circle"
	IconNameCaretUp                  IconName = "caret-up"
	IconNameCart                     IconName = "cart"
	IconNameCash                     IconName = "cash"
	IconNameCellular                 IconName = "cellular"
	IconNameChatboxEllipses          IconName = "chatbox-ellipses"
	IconNameChatbox                  IconName = "chatbox"
	IconNameChatbubbleEllipses       IconName = "chatbubble-ellipses"
	IconNameChatbubble               IconName = "chatbubble"
	IconNameChatbubbles              IconName = "chatbubbles"
	IconNameCheckbox                 IconName = "checkbox"
	IconNameCheckmarkCircle          IconName = "checkmark-circle"
	IconNameCheckmarkDoneCircle      IconName = "checkmark-done-circle"
	IconNameChevronBackCircle        IconName = "chevron-back-circle"
	IconNameChevronDownCircle        IconName = "chevron-down-circle"
	IconNameChevronForwardCircle     IconName = "chevron-forward-circle"
	IconNameChevronUpCircle          IconName = "chevron-up-circle"
	IconNameClipboard                IconName = "clipboard"
	IconNameCloseCircle              IconName = "close-circle"
	IconNameCloudCircle              IconName = "cloud-circle"
	IconNameCloudDone                IconName = "cloud-done"
	IconNameCloudDownload            IconName = "cloud-download"
	IconNameCloudOffline             IconName = "cloud-offline"
	IconNameCloudUpload              IconName = "cloud-upload"
	IconNameCloud                    IconName = "cloud"
	IconNameCloudyNight              IconName = "cloudy-night"
	IconNameCloudy                   IconName = "cloudy"
	IconNameCodeSlash                IconName = "code-slash"
	IconNameCode                     IconName = "code"
	IconNameCog                      IconName = "cog"
	IconNameColorFill                IconName = "color-fill"
	IconNameColorFilter              IconName = "color-filter"
	IconNameColorPalette             IconName = "color-palette"
	IconNameColorWand                IconName = "color-wand"
	IconNameCompass                  IconName = "compass"
	IconNameConstruct                IconName = "construct"
	IconNameContact                  IconName = "contact"
	IconNameContract                 IconName = "contract"
	IconNameContrast                 IconName = "contrast"
	IconNameCopy                     IconName = "copy"
	IconNameCreate                   IconName = "create"
	IconNameCrop                     IconName = "crop"
	IconNameCube                     IconName = "cube"
	IconNameCut                      IconName = "cut"
	IconNameDesktop                  IconName = "desktop"
	IconNameDiamond                  IconName = "diamond"
	IconNameDice                     IconName = "dice"
	IconNameDisc                     IconName = "disc"
	IconNameDocumentAttach           IconName = "document-attach"
	IconNameDocumentLock             IconName = "document-lock"
	IconNameDocumentText             IconName = "document-text"
	IconNameDocument                 IconName = "document"
	IconNameDocuments                IconName = "documents"
	IconNameDownload                 IconName = "download"
	IconNameDuplicate                IconName = "duplicate"
	IconNameEar                      IconName = "ear"
	IconNameEarth                    IconName = "earth"
	IconNameEasel                    IconName = "easel"
	IconNameEgg                      IconName = "egg"
	IconNameEllipse                  IconName = "ellipse"
	IconNameEllipsisHorizontalCircle IconName = "ellipsis-horizontal-circle"
	IconNameEllipsisVerticalCircle   IconName = "ellipsis-vertical-circle"
	IconNameEnter                    IconName = "enter"
	IconNameExit                     IconName = "exit"
	IconNameExpand                   IconName = "expand"
	IconNameExtensionPuzzle          IconName = "extension-puzzle"
	IconNameEyeOff                   IconName = "eye-off"
	IconNameEye                      IconName = "eye"
	IconNameEyedrop                  IconName = "eyedrop"
	IconNameFastFood                 IconName = "fast-food"
	IconNameFemale                   IconName = "female"
	IconNameFileTrayFull             IconName = "file-tray-full"
	IconNameFileTrayStacked          IconName = "file-tray-stacked"
	IconNameFileTray                 IconName = "file-tray"
	IconNameFilm                     IconName = "film"
	IconNameFilterCircle             IconName = "filter-circle"
	IconNameFingerPrint              IconName = "finger-print"
	IconNameFish                     IconName = "fish"
	IconNameFitness                  IconName = "fitness"
	IconNameFlag                     IconName = "flag"
	IconNameFlame                    IconName = "flame"
	IconNameFlashOff                 IconName = "flash-off"
	IconNameFlash                    IconName = "flash"
	IconNameFlashlight               IconName = "flashlight"
	IconNameFlask                    IconName = "flask"
	IconNameFlower                   IconName = "flower"
	IconNameFolderOpen               IconName = "folder-open"
	IconNameFolder                   IconName = "folder"
	IconNameFootball                 IconName = "football"
	IconNameFootsteps                IconName = "footsteps"
	IconNameFunnel                   IconName = "funnel"
	IconNameGameController           IconName = "game-controller"
	IconNameGift                     IconName = "gift"
	IconNameGitBranch                IconName = "git-branch"
	IconNameGitCommit                IconName = "git-commit"
	IconNameGitCompare               IconName = "git-compare"
	IconNameGitMerge                 IconName = "git-merge"
	IconNameGitNetwork               IconName = "git-network"
	IconNameGitPullRequest           IconName = "git-pull-request"
	IconNameGlasses                  IconName = "glasses"
	IconNameGlobe                    IconName = "globe"
	IconNameGolf                     IconName = "golf"
	IconNameGrid                     IconName = "grid"
	IconNameHammer                   IconName = "hammer"
	IconNameHandLeft                 IconName = "hand-left"
	IconNameHandRight                IconName = "hand-right"
	IconNameHappy                    IconName = "happy"
	IconNameHardwareChip             IconName = "hardware-chip"
	IconNameHeadset                  IconName = "headset"
	IconNameHeartCircle              IconName = "heart-circle"
	IconNameHeartDislikeCircle       IconName = "heart-dislike-circle"
	IconNameHeartDislike             IconName = "heart-dislike"
	IconNameHeartHalf                IconName = "heart-half"
	IconNameHeart                    IconName = "heart"
	IconNameHelpBuoy                 IconName = "help-buoy"
	IconNameHelpCircle               IconName = "help-circle"
	IconNameHome                     IconName = "home"
	IconNameHourglass                IconName = "hourglass"
	IconNameIceCream                 IconName = "ice-cream"
	IconNameIdCard                   IconName = "id-card"
	IconNameImage                    IconName = "image"
	IconNameImages                   IconName = "images"
	IconNameInfinite                 IconName = "infinite"
	IconNameInformationCircle        IconName = "information-circle"
	IconNameInvertMode               IconName = "invert-mode"
	IconNameJournal                  IconName = "journal"
	IconNameKey                      IconName = "key"
	IconNameKeypad                   IconName = "keypad"
	IconNameLanguage                 IconName = "language"
	IconNameLaptop                   IconName = "laptop"
	IconNameLayers                   IconName = "layers"
	IconNameLeaf                     IconName = "leaf"
	IconNameLibrary                  IconName = "library"
	IconNameLink                     IconName = "link"
	IconNameListCircle               IconName = "list-circle"
	IconNameList                     IconName = "list"
	IconNameLocate                   IconName = "locate"
	IconNameLocation                 IconName = "location"
	IconNameLockClosed               IconName = "lock-closed"
	IconNameLockOpen                 IconName = "lock-open"
	IconNameLogIn                    IconName = "log-in"
	IconNameLogOut                   IconName = "log-out"
	IconNameLogoAlipay               IconName = "logo-alipay"
	IconNameLogoAmazon               IconName = "logo-amazon"
	IconNameLogoAmplify              IconName = "logo-amplify"
	IconNameLogoAndroid              IconName = "logo-android"
	IconNameMagnet                   IconName = "magnet"
	IconNameMailOpen                 IconName = "mail-open"
	IconNameMailUnread               IconName = "mail-unread"
	IconNameMail                     IconName = "mail"
	IconNameMaleFemale               IconName = "male-female"
	IconNameMale                     IconName = "male"
	IconNameMan                      IconName = "man"
	IconNameMap                      IconName = "map"
	IconNameMedal                    IconName = "medal"
	IconNameMedical                  IconName = "medical"
	IconNameMedkit                   IconName = "medkit"
	IconNameMegaphone                IconName = "megaphone"
	IconNameMenu                     IconName = "menu"
	IconNameMicCircle                IconName = "mic-circle"
	IconNameMicOffCircle             IconName = "mic-off-circle"
	IconNameMicOff                   IconName = "mic-off"
	IconNameMic                      IconName = "mic"
	IconNameMoon                     IconName = "moon"
	IconNameMove                     IconName = "move"
	IconNameMusicalNote              IconName = "musical-note"
	IconNameMusicalNotes             IconName = "musical-notes"
	IconNameNavigateCircle           IconName = "navigate-circle"
	IconNameNavigate                 IconName = "navigate"
	IconNameNewspaper                IconName = "newspaper"
	IconNameNotificationsCircle      IconName = "notifications-circle"
	IconNameNotificationsOffCircle   IconName = "notifications-off-circle"
	IconNameNotificationsOff         IconName = "notifications-off"
	IconNameNotifications            IconName = "notifications"
	IconNameNuclear                  IconName = "nuclear"
	IconNameNutrition                IconName = "nutrition"
	IconNameOptions                  IconName = "options"
	IconNamePaperPlane               IconName = "paper-plane"
	IconNamePartlySunny              IconName = "partly-sunny"
	IconNamePauseCircle              IconName = "pause-circle"
	IconNamePause                    IconName = "pause"
	IconNamePaw                      IconName = "paw"
	IconNamePencil                   IconName = "pencil"
	IconNamePeopleCircle             IconName = "people-circle"
	IconNamePeople                   IconName = "people"
	IconNamePersonAdd                IconName = "person-add"
	IconNamePersonCircle             IconName = "person-circle"
	IconNamePersonRemove             IconName = "person-remove"
	IconNamePerson                   IconName = "person"
	IconNamePhoneLandscape           IconName = "phone-landscape"
	IconNamePhonePortrait            IconName = "phone-portrait"
	IconNamePieChart                 IconName = "pie-chart"
	IconNamePin                      IconName = "pin"
	IconNamePint                     IconName = "pint"
	IconNamePizza                    IconName = "pizza"
	IconNamePlanet                   IconName = "planet"
	IconNamePlayBackCircle           IconName = "play-back-circle"
	IconNamePlayBack                 IconName = "play-back"
	IconNamePlayCircle               IconName = "play-circle"
	IconNamePlayForwardCircle        IconName = "play-forward-circle"
	IconNamePlayForward              IconName = "play-forward"
	IconNamePlaySkipBackCircle       IconName = "play-skip-back-circle"
	IconNamePlaySkipBack             IconName = "play-skip-back"
	IconNamePlaySkipForwardCircle    IconName = "play-skip-forward-circle"
	IconNamePlaySkipForward          IconName = "play-skip-forward"
	IconNamePlay                     IconName = "play"
	IconNamePodium                   IconName = "podium"
	IconNamePower                    IconName = "power"
	IconNamePricetag                 IconName = "pricetag"
	IconNamePricetags                IconName = "pricetags"
	IconNamePrint                    IconName = "print"
	IconNamePrism                    IconName = "prism"
	IconNamePulse                    IconName = "pulse"
	IconNamePush                     IconName = "push"
	IconNameQrCode                   IconName = "qr-code"
	IconNameRadioButtonOff           IconName = "radio-button-off"
	IconNameRadioButtonOn            IconName = "radio-button-on"
	IconNameRadio                    IconName = "radio"
	IconNameRainy                    IconName = "rainy"
	IconNameReader                   IconName = "reader"
	IconNameReceipt                  IconName = "receipt"
	IconNameRecording                IconName = "recording"
	IconNameRefreshCircle            IconName = "refresh-circle"
	IconNameRefresh                  IconName = "refresh"
	IconNameReloadCircle             IconName = "reload-circle"
	IconNameReload                   IconName = "reload"
	IconNameRemoveCircle             IconName = "remove-circle"
	IconNameRepeat                   IconName = "repeat"
	IconNameResize                   IconName = "resize"
	IconNameRestaurant               IconName = "restaurant"
	IconNameRibbon                   IconName = "ribbon"
	IconNameRocket                   IconName = "rocket"
	IconNameRose                     IconName = "rose"
	IconNameSad                      IconName = "sad"
	IconNameSave                     IconName = "save"
	IconNameScale                    IconName = "scale"
	IconNameScanCircle               IconName = "scan-circle"
	IconNameScan                     IconName = "scan"
	IconNameSchool                   IconName = "school"
	IconNameSearchCircle             IconName = "search-circle"
	IconNameSearch                   IconName = "search"
	IconNameSend                     IconName = "send"
	IconNameServer                   IconName = "server"
	IconNameSettings                 IconName = "settings"
	IconNameShapes                   IconName = "shapes"
	IconNameShareSocial              IconName = "share-social"
	IconNameShare                    IconName = "share"
	IconNameShieldCheckmark          IconName = "shield-checkmark"
	IconNameShieldHalf               IconName = "shield-half"
	IconNameShield                   IconName = "shield"
	IconNameShirt                    IconName = "shirt"
	IconNameShuffle                  IconName = "shuffle"
	IconNameSkull                    IconName = "skull"
	IconNameSnow                     IconName = "snow"
	IconNameSparkles                 IconName = "sparkles"
	IconNameSpeedometer              IconName = "speedometer"
	IconNameSquare                   IconName = "square"
	IconNameStarHalf                 IconName = "star-half"
	IconNameStar                     IconName = "star"
	IconNameStatsChart               IconName = "stats-chart"
	IconNameStopCircle               IconName = "stop-circle"
	IconNameStop                     IconName = "stop"
	IconNameStopwatch                IconName = "stopwatch"
	IconNameStorefront               IconName = "storefront"
	IconNameSubway                   IconName = "subway"
	IconNameSunny                    IconName = "sunny"
	IconNameSwapHorizontal           IconName = "swap-horizontal"
	IconNameSwapVertical             IconName = "swap-vertical"
	IconNameSyncCircle               IconName = "sync-circle"
	IconNameSync                     IconName = "sync"
	IconNameTabletLandscape          IconName = "tablet-landscape"
	IconNameTabletPortrait           IconName = "tablet-portrait"
	IconNameTelescope                IconName = "telescope"
	IconNameTennisball               IconName = "tennisball"
	IconNameTerminal                 IconName = "terminal"
	IconNameText                     IconName = "text"
	IconNameThermometer              IconName = "thermometer"
	IconNameThumbsDown               IconName = "thumbs-down"
	IconNameThumbsUp                 IconName = "thumbs-up"
	IconNameThunderstorm             IconName = "thunderstorm"
	IconNameTicket                   IconName = "ticket"
	IconNameTime                     IconName = "time"
	IconNameTimer                    IconName = "timer"
	IconNameToday                    IconName = "today"
	IconNameToggle                   IconName = "toggle"
	IconNameTrailSign                IconName = "trail-sign"
	IconNameTrain                    IconName = "train"
	IconNameTransgender              IconName = "transgender"
	IconNameTrashBin                 IconName = "trash-bin"
	IconNameTrash                    IconName = "trash"
	IconNameTrendingDown             IconName = "trending-down"
	IconNameTrendingUp               IconName = "trending-up"
	IconNameTriangle                 IconName = "triangle"
	IconNameTrophy                   IconName = "trophy"
	IconNameTv                       IconName = "tv"
	IconNameUmbrella                 IconName = "umbrella"
	IconNameUnlink                   IconName = "unlink"
	IconNameVideocamOff              IconName = "videocam-off"
	IconNameVideocam                 IconName = "videocam"
	IconNameVolumeHigh               IconName = "volume-high"
	IconNameVolumeLow                IconName = "volume-low"
	IconNameVolumeMedium             IconName = "volume-medium"
	IconNameVolumeMute               IconName = "volume-mute"
	IconNameVolumeOff                IconName = "volume-off"
	IconNameWalk                     IconName = "walk"
	IconNameWallet                   IconName = "wallet"
	IconNameWarning                  IconName = "warning"
	IconNameWatch                    IconName = "watch"
	IconNameWater                    IconName = "water"
	IconNameWifi                     IconName = "wifi"
	IconNameWine                     IconName = "wine"
	IconNameWoman                    IconName = "woman"
)

var validIconNames = func() map[IconName]struct{} {
	m := make(map[IconName]struct{}, 390)
	for _, v := range []IconName{
		IconNameAccessibility,
		IconNameAddCircle,
		IconNameAirplane,
		IconNameAlarm,
		IconNameAlbums,
		IconNameAlertCircle,
		IconNameAmericanFootball,
		IconNameAnalytics,
		IconNameAperture,
		IconNameApps,
		IconNameArchive,
		IconNameArrowBackCircle,
		IconNameArrowDownCircle,
		IconNameArrowForwardCircle,
		IconNameArrowRedoCircle,
		IconNameArrowRedo,
		IconNameArrowUndoCircle,
		IconNameArrowUndo,
		IconNameArrowUpCircle,
		IconNameAtCircle,
		IconNameAttach,
		IconNameBackspace,
		IconNameBagAdd,
		IconNameBagCheck,
		IconNameBagHandle,
		IconNameBagRemove,
		IconNameBag,
		IconNameBalloon,
		IconNameBan,
		IconNameBandage,
		IconNameBarChart,
		IconNameBarbell,
		IconNameBarcode,
		IconNameBaseball,
		IconNameBasket,
		IconNameBasketball,
		IconNameBatteryCharging,
		IconNameBatteryDead,
		IconNameBatteryFull,
		IconNameBatteryHalf,
		IconNameBeaker,
		IconNameBed,
		IconNameBeer,
		IconNameBicycle,
		IconNameBinoculars,
		IconNameBluetooth,
		IconNameBoat,
		IconNameBody,
		IconNameBonfire,
		IconNameBook,
		IconNameBookmark,
		IconNameBookmarks,
		IconNameBowlingBall,
		IconNameBriefcase,
		IconNameBrowsers,
		IconNameBrush,
		IconNameBug,
		IconNameBuild,
		IconNameBulb,
		IconNameBus,
		IconNameBusiness,
		IconNameCafe,
		IconNameCalculator,
		IconNameCalendarClear,
		IconNameCalendarNumber,
		IconNameCalendar,
		IconNameCall,
		IconNameCameraReverse,
		IconNameCamera,
		IconNameCarSport,
		IconNameCar,
		IconNameCard,
		IconNameCaretBackCircle,
		IconNameCaretBack,
		IconNameCaretDownCircle,
		IconNameCaretDown,
		IconNameCaretForwardCircle,
		IconNameCaretForward,
		IconNameCaretUpCircle,
		IconNameCaretUp,
		IconNameCart,
		IconNameCash,
		IconNameCellular,
		IconNameChatboxEllipses,
		IconNameChatbox,
		IconNameChatbubbleEllipses,
		IconNameChatbubble,
		IconNameChatbubbles,
		IconNameCheckbox,
		IconNameCheckmarkCircle,
		IconNameCheckmarkDoneCircle,
		IconNameChevronBackCircle,
		IconNameChevronDownCircle,
		IconNameChevronForwardCircle,
		IconNameChevronUpCircle,
		IconNameClipboard,
		IconNameCloseCircle,
		IconNameCloudCircle,
		IconNameCloudDone,
		IconNameCloudDownload,
		IconNameCloudOffline,
		IconNameCloudUpload,
		IconNameCloud,
		IconNameCloudyNight,
		IconNameCloudy,
		IconNameCodeSlash,
		IconNameCode,
		IconNameCog,
		IconNameColorFill,
		IconNameColorFilter,
		IconNameColorPalette,
		IconNameColorWand,
		IconNameCompass,
		IconNameConstruct,
		IconNameContact,
		IconNameContract,
		IconNameContrast,
		IconNameCopy,
		IconNameCreate,
		IconNameCrop,
		IconNameCube,
		IconNameCut,
		IconNameDesktop,
		IconNameDiamond,
		IconNameDice,
		IconNameDisc,
		IconNameDocumentAttach,
		IconNameDocumentLock,
		IconNameDocumentText,
		IconNameDocument,
		IconNameDocuments,
		IconNameDownload,
		IconNameDuplicate,
		IconNameEar,
		IconNameEarth,
		IconNameEasel,
		IconNameEgg,
		IconNameEllipse,
		IconNameEllipsisHorizontalCircle,
		IconNameEllipsisVerticalCircle,
		IconNameEnter,
		IconNameExit,
		IconNameExpand,
		IconNameExtensionPuzzle,
		IconNameEyeOff,
		IconNameEye,
		IconNameEyedrop,
		IconNameFastFood,
		IconNameFemale,
		IconNameFileTrayFull,
		IconNameFileTrayStacked,
		IconNameFileTray,
		IconNameFilm,
		IconNameFilterCircle,
		IconNameFingerPrint,
		IconNameFish,
		IconNameFitness,
		IconNameFlag,
		IconNameFlame,
		IconNameFlashOff,
		IconNameFlash,
		IconNameFlashlight,
		IconNameFlask,
		IconNameFlower,
		IconNameFolderOpen,
		IconNameFolder,
		IconNameFootball,
		IconNameFootsteps,
		IconNameFunnel,
		IconNameGameController,
		IconNameGift,
		IconNameGitBranch,
		IconNameGitCommit,
		IconNameGitCompare,
		IconNameGitMerge,
		IconNameGitNetwork,
		IconNameGitPullRequest,
		IconNameGlasses,
		IconNameGlobe,
		IconNameGolf,
		IconNameGrid,
		IconNameHammer,
		IconNameHandLeft,
		IconNameHandRight,
		IconNameHappy,
		IconNameHardwareChip,
		IconNameHeadset,
		IconNameHeartCircle,
		IconNameHeartDislikeCircle,
		IconNameHeartDislike,
		IconNameHeartHalf,
		IconNameHeart,
		IconNameHelpBuoy,
		IconNameHelpCircle,
		IconNameHome,
		IconNameHourglass,
		IconNameIceCream,
		IconNameIdCard,
		IconNameImage,
		IconNameImages,
		IconNameInfinite,
		IconNameInformationCircle,
		IconNameInvertMode,
		IconNameJournal,
		IconNameKey,
		IconNameKeypad,
		IconNameLanguage,
		IconNameLaptop,
		IconNameLayers,
		IconNameLeaf,
		IconNameLibrary,
		IconNameLink,
		IconNameListCircle,
		IconNameList,
		IconNameLocate,
		IconNameLocation,
		IconNameLockClosed,
		IconNameLockOpen,
		IconNameLogIn,
		IconNameLogOut,
		IconNameLogoAlipay,
		IconNameLogoAmazon,
		IconNameLogoAmplify,
		IconNameLogoAndroid,
		IconNameMagnet,
		IconNameMailOpen,
		IconNameMailUnread,
		IconNameMail,
		IconNameMaleFemale,
		IconNameMale,
		IconNameMan,
		IconNameMap,
		IconNameMedal,
		IconNameMedical,
		IconNameMedkit,
		IconNameMegaphone,
		IconNameMenu,
		IconNameMicCircle,
		IconNameMicOffCircle,
		IconNameMicOff,
		IconNameMic,
		IconNameMoon,
		IconNameMove,
		IconNameMusicalNote,
		IconNameMusicalNotes,
		IconNameNavigateCircle,
		IconNameNavigate,
		IconNameNewspaper,
		IconNameNotificationsCircle,
		IconNameNotificationsOffCircle,
		IconNameNotificationsOff,
		IconNameNotifications,
		IconNameNuclear,
		IconNameNutrition,
		IconNameOptions,
		IconNamePaperPlane,
		IconNamePartlySunny,
		IconNamePauseCircle,
		IconNamePause,
		IconNamePaw,
		IconNamePencil,
		IconNamePeopleCircle,
		IconNamePeople,
		IconNamePersonAdd,
		IconNamePersonCircle,
		IconNamePersonRemove,
		IconNamePerson,
		IconNamePhoneLandscape,
		IconNamePhonePortrait,
		IconNamePieChart,
		IconNamePin,
		IconNamePint,
		IconNamePizza,
		IconNamePlanet,
		IconNamePlayBackCircle,
		IconNamePlayBack,
		IconNamePlayCircle,
		IconNamePlayForwardCircle,
		IconNamePlayForward,
		IconNamePlaySkipBackCircle,
		IconNamePlaySkipBack,
		IconNamePlaySkipForwardCircle,
		IconNamePlaySkipForward,
		IconNamePlay,
		IconNamePodium,
		IconNamePower,
		IconNamePricetag,
		IconNamePricetags,
		IconNamePrint,
		IconNamePrism,
		IconNamePulse,
		IconNamePush,
		IconNameQrCode,
		IconNameRadioButtonOff,
		IconNameRadioButtonOn,
		IconNameRadio,
		IconNameRainy,
		IconNameReader,
		IconNameReceipt,
		IconNameRecording,
		IconNameRefreshCircle,
		IconNameRefresh,
		IconNameReloadCircle,
		IconNameReload,
		IconNameRemoveCircle,
		IconNameRepeat,
		IconNameResize,
		IconNameRestaurant,
		IconNameRibbon,
		IconNameRocket,
		IconNameRose,
		IconNameSad,
		IconNameSave,
		IconNameScale,
		IconNameScanCircle,
		IconNameScan,
		IconNameSchool,
		IconNameSearchCircle,
		IconNameSearch,
		IconNameSend,
		IconNameServer,
		IconNameSettings,
		IconNameShapes,
		IconNameShareSocial,
		IconNameShare,
		IconNameShieldCheckmark,
		IconNameShieldHalf,
		IconNameShield,
		IconNameShirt,
		IconNameShuffle,
		IconNameSkull,
		IconNameSnow,
		IconNameSparkles,
		IconNameSpeedometer,
		IconNameSquare,
		IconNameStarHalf,
		IconNameStar,
		IconNameStatsChart,
		IconNameStopCircle,
		IconNameStop,
		IconNameStopwatch,
		IconNameStorefront,
		IconNameSubway,
		IconNameSunny,
		IconNameSwapHorizontal,
		IconNameSwapVertical,
		IconNameSyncCircle,
		IconNameSync,
		IconNameTabletLandscape,
		IconNameTabletPortrait,
		IconNameTelescope,
		IconNameTennisball,
		IconNameTerminal,
		IconNameText,
		IconNameThermometer,
		IconNameThumbsDown,
		IconNameThumbsUp,
		IconNameThunderstorm,
		IconNameTicket,
		IconNameTime,
		IconNameTimer,
		IconNameToday,
		IconNameToggle,
		IconNameTrailSign,
		IconNameTrain,
		IconNameTransgender,
		IconNameTrashBin,
		IconNameTrash,
		IconNameTrendingDown,
		IconNameTrendingUp,
		IconNameTriangle,
		IconNameTrophy,
		IconNameTv,
		IconNameUmbrella,
		IconNameUnlink,
		IconNameVideocamOff,
		IconNameVideocam,
		IconNameVolumeHigh,
		IconNameVolumeLow,
		IconNameVolumeMedium,
		IconNameVolumeMute,
		IconNameVolumeOff,
		IconNameWalk,
		IconNameWallet,
		IconNameWarning,
		IconNameWatch,
		IconNameWater,
		IconNameWifi,
		IconNameWine,
		IconNameWoman,
	} {
		m[v] = struct{}{}
	}
	return m
}()

func (n *IconName) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	name := IconName(s)
	if _, ok := validIconNames[name]; !ok {
		return util.ErrBadInput(fmt.Sprintf("invalid icon name: %q", s))
	}
	*n = name
	return nil
}

var IconOptionToColor = map[float64]Color{
	1:  ColorGrey,
	2:  ColorYellow,
	3:  ColorOrange,
	4:  ColorRed,
	5:  ColorPink,
	6:  ColorPurple,
	7:  ColorBlue,
	8:  ColorIce,
	9:  ColorTeal,
	10: ColorLime,
}

var ColorToIconOption = map[Color]int64{
	ColorGrey:   1,
	ColorYellow: 2,
	ColorOrange: 3,
	ColorRed:    4,
	ColorPink:   5,
	ColorPurple: 6,
	ColorBlue:   7,
	ColorIce:    8,
	ColorTeal:   9,
	ColorLime:   10,
}

var ColorOptionToColor = map[string]Color{
	"grey":   ColorGrey,
	"yellow": ColorYellow,
	"orange": ColorOrange,
	"red":    ColorRed,
	"pink":   ColorPink,
	"purple": ColorPurple,
	"blue":   ColorBlue,
	"ice":    ColorIce,
	"teal":   ColorTeal,
	"lime":   ColorLime,
}

var ColorToColorOption = map[Color]string{
	ColorGrey:   "grey",
	ColorYellow: "yellow",
	ColorOrange: "orange",
	ColorRed:    "red",
	ColorPink:   "pink",
	ColorPurple: "purple",
	ColorBlue:   "blue",
	ColorIce:    "ice",
	ColorTeal:   "teal",
	ColorLime:   "lime",
}

type Icon struct {
	WrappedIcon `swaggerignore:"true"`
}

func (i Icon) MarshalJSON() ([]byte, error) {
	return json.Marshal(i.WrappedIcon)
}

func (i *Icon) UnmarshalJSON(data []byte) error {
	var raw struct {
		Format IconFormat `json:"format"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	switch raw.Format {
	case IconFormatEmoji:
		var emojiIcon EmojiIcon
		if err := json.Unmarshal(data, &emojiIcon); err != nil {
			return err
		}
		i.WrappedIcon = emojiIcon
	case IconFormatFile:
		var fileIcon FileIcon
		if err := json.Unmarshal(data, &fileIcon); err != nil {
			return err
		}
		i.WrappedIcon = fileIcon
	case IconFormatIcon:
		var namedIcon NamedIcon
		if err := json.Unmarshal(data, &namedIcon); err != nil {
			return err
		}
		i.WrappedIcon = namedIcon
	default:
		return util.ErrBadInput(fmt.Sprintf("invalid icon format: %q", raw.Format))
	}
	return nil
}

type WrappedIcon interface{ isIcon() }

type EmojiIcon struct {
	Format IconFormat `json:"format" enums:"emoji"` // The format of the icon
	Emoji  string     `json:"emoji" example:"📄"`    // The emoji of the icon
}

func (EmojiIcon) isIcon() {}

type FileIcon struct {
	Format IconFormat `json:"format" enums:"file"`                                                        // The format of the icon
	File   string     `json:"file" example:"bafybeieptz5hvcy6txplcvphjbbh5yjc2zqhmihs3owkh5oab4ezauzqay"` // The file of the icon
}

func (FileIcon) isIcon() {}

// TODO: the enum gen for IconFormat through swaggo is bugged; only the last enum (before: "icon") is used
type NamedIcon struct {
	Format IconFormat `json:"format" enums:"emoji,file,icon"`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         // The format of the icon
	Name   IconName   `json:"name" enums:"accessibility,add-circle,airplane,alarm,albums,alert-circle,american-football,analytics,aperture,apps,archive,arrow-back-circle,arrow-down-circle,arrow-forward-circle,arrow-redo-circle,arrow-redo,arrow-undo-circle,arrow-undo,arrow-up-circle,at-circle,attach,backspace,bag-add,bag-check,bag-handle,bag-remove,bag,balloon,ban,bandage,bar-chart,barbell,barcode,baseball,basket,basketball,battery-charging,battery-dead,battery-full,battery-half,beaker,bed,beer,bicycle,binoculars,bluetooth,boat,body,bonfire,book,bookmark,bookmarks,bowling-ball,briefcase,browsers,brush,bug,build,bulb,bus,business,cafe,calculator,calendar-clear,calendar-number,calendar,call,camera-reverse,camera,car-sport,car,card,caret-back-circle,caret-back,caret-down-circle,caret-down,caret-forward-circle,caret-forward,caret-up-circle,caret-up,cart,cash,cellular,chatbox-ellipses,chatbox,chatbubble-ellipses,chatbubble,chatbubbles,checkbox,checkmark-circle,checkmark-done-circle,chevron-back-circle,chevron-down-circle,chevron-forward-circle,chevron-up-circle,clipboard,close-circle,cloud-circle,cloud-done,cloud-download,cloud-offline,cloud-upload,cloud,cloudy-night,cloudy,code-slash,code,cog,color-fill,color-filter,color-palette,color-wand,compass,construct,contact,contract,contrast,copy,create,crop,cube,cut,desktop,diamond,dice,disc,document-attach,document-lock,document-text,document,documents,download,duplicate,ear,earth,easel,egg,ellipse,ellipsis-horizontal-circle,ellipsis-vertical-circle,enter,exit,expand,extension-puzzle,eye-off,eye,eyedrop,fast-food,female,file-tray-full,file-tray-stacked,file-tray,film,filter-circle,finger-print,fish,fitness,flag,flame,flash-off,flash,flashlight,flask,flower,folder-open,folder,football,footsteps,funnel,game-controller,gift,git-branch,git-commit,git-compare,git-merge,git-network,git-pull-request,glasses,globe,golf,grid,hammer,hand-left,hand-right,happy,hardware-chip,headset,heart-circle,heart-dislike-circle,heart-dislike,heart-half,heart,help-buoy,help-circle,home,hourglass,ice-cream,id-card,image,images,infinite,information-circle,invert-mode,journal,key,keypad,language,laptop,layers,leaf,library,link,list-circle,list,locate,location,lock-closed,lock-open,log-in,log-out,logo-alipay,logo-amazon,logo-amplify,logo-android,magnet,mail-open,mail-unread,mail,male-female,male,man,map,medal,medical,medkit,megaphone,menu,mic-circle,mic-off-circle,mic-off,mic,moon,move,musical-note,musical-notes,navigate-circle,navigate,newspaper,notifications-circle,notifications-off-circle,notifications-off,notifications,nuclear,nutrition,options,paper-plane,partly-sunny,pause-circle,pause,paw,pencil,people-circle,people,person-add,person-circle,person-remove,person,phone-landscape,phone-portrait,pie-chart,pin,pint,pizza,planet,play-back-circle,play-back,play-circle,play-forward-circle,play-forward,play-skip-back-circle,play-skip-back,play-skip-forward-circle,play-skip-forward,play,podium,power,pricetag,pricetags,print,prism,pulse,push,qr-code,radio-button-off,radio-button-on,radio,rainy,reader,receipt,recording,refresh-circle,refresh,reload-circle,reload,remove-circle,repeat,resize,restaurant,ribbon,rocket,rose,sad,save,scale,scan-circle,scan,school,search-circle,search,send,server,settings,shapes,share-social,share,shield-checkmark,shield-half,shield,shirt,shuffle,skull,snow,sparkles,speedometer,square,star-half,star,stats-chart,stop-circle,stop,stopwatch,storefront,subway,sunny,swap-horizontal,swap-vertical,sync-circle,sync,t.txt,tablet-landscape,tablet-portrait,telescope,tennisball,terminal,text,thermometer,thumbs-down,thumbs-up,thunderstorm,ticket,time,timer,today,toggle,trail-sign,train,transgender,trash-bin,trash,trending-down,trending-up,triangle,trophy,tv,umbrella,unlink,videocam-off,videocam,volume-high,volume-low,volume-medium,volume-mute,volume-off,walk,wallet,warning,watch,water,wifi,wine,woman"` // The name of the icon
	Color  Color      `json:"color" example:"yellow" enums:"grey,yellow,orange,red,pink,purple,blue,ice,teal,lime"`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   // The color of the icon
}

func (NamedIcon) isIcon() {}
