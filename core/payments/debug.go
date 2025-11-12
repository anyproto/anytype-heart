package payments

import (
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"go.uber.org/zap"
)

// DEBUG functions
func logV2DataDiff(cachedData, fetchedMembership *model.MembershipV2Data) {
	log := logging.Logger("payments-diff")

	log.Info("=== V2 Data Diff Analysis ===")

	// Handle nil cases
	if cachedData == nil && fetchedMembership == nil {
		log.Info("Both data objects are nil")
		return
	}
	if cachedData == nil || fetchedMembership == nil {
		log.Warn("One data object is nil", zap.Bool("cachedNil", cachedData == nil), zap.Bool("fetchedNil", fetchedMembership == nil))
		return
	}

	// Compare Products
	log.Info("--- Products Comparison ---")
	compareProducts(log, cachedData.Products, fetchedMembership.Products)

	// Compare NextInvoice
	log.Info("--- NextInvoice Comparison ---")
	compareNextInvoice(log, cachedData.NextInvoice, fetchedMembership.NextInvoice)

	// Compare TeamOwnerID
	if cachedData.TeamOwnerID != fetchedMembership.TeamOwnerID {
		log.Warn("TeamOwnerID differs", zap.String("cached", cachedData.TeamOwnerID), zap.String("fetched", fetchedMembership.TeamOwnerID))
	} else {
		log.Info("TeamOwnerID matches", zap.String("value", cachedData.TeamOwnerID))
	}

	// Compare PaymentProvider
	if cachedData.PaymentProvider != fetchedMembership.PaymentProvider {
		log.Warn("PaymentProvider differs", zap.Int32("cached", int32(cachedData.PaymentProvider)), zap.Int32("fetched", int32(fetchedMembership.PaymentProvider)))
	} else {
		log.Info("PaymentProvider matches", zap.Int32("value", int32(cachedData.PaymentProvider)))
	}

	log.Info("=== End V2 Data Diff Analysis ===")
}

func compareProducts(log *logging.Sugared, cached []*model.MembershipV2PurchasedProduct, fetched []*model.MembershipV2PurchasedProduct) {
	// Handle nil/empty cases
	cachedNil := cached == nil
	fetchedNil := fetched == nil
	cachedEmpty := len(cached) == 0
	fetchedEmpty := len(fetched) == 0

	if cachedNil && fetchedNil {
		log.Info("Both product slices are nil")
		return
	}
	if cachedNil && fetchedEmpty {
		log.Info("Cached products nil, fetched empty")
		return
	}
	if cachedEmpty && fetchedNil {
		log.Info("Cached products empty, fetched nil")
		return
	}
	if cachedNil || fetchedNil {
		log.Warn("Product slice nil status differs", zap.Bool("cachedNil", cachedNil), zap.Bool("fetchedNil", fetchedNil))
		return
	}

	if len(cached) != len(fetched) {
		log.Warn("Product slice lengths differ", zap.Int("cachedLen", len(cached)), zap.Int("fetchedLen", len(fetched)))
		return
	}

	log.Info("Comparing products", zap.Int("count", len(cached)))
	for i := range cached {
		log.Info("--- Product comparison ---", zap.Int("index", i))
		comparePurchasedProduct(log, cached[i], fetched[i])
	}
}

func comparePurchasedProduct(log *logging.Sugared, cached *model.MembershipV2PurchasedProduct, fetched *model.MembershipV2PurchasedProduct) {
	if cached == nil && fetched == nil {
		log.Info("Both purchased products are nil")
		return
	}
	if cached == nil || fetched == nil {
		log.Warn("Purchased product nil status differs", zap.Bool("cachedNil", cached == nil), zap.Bool("fetchedNil", fetched == nil))
		return
	}

	// Compare Product field
	compareProduct(log, cached.Product, fetched.Product)

	// Compare PurchaseInfo field
	comparePurchaseInfo(log, cached.PurchaseInfo, fetched.PurchaseInfo)

	// Compare ProductStatus field
	compareProductStatus(log, cached.ProductStatus, fetched.ProductStatus)
}

func compareProduct(log *logging.Sugared, cached *model.MembershipV2Product, fetched *model.MembershipV2Product) {
	if cached == nil && fetched == nil {
		log.Info("Both products are nil")
		return
	}
	if cached == nil || fetched == nil {
		log.Warn("Product nil status differs", zap.Bool("cachedNil", cached == nil), zap.Bool("fetchedNil", fetched == nil))
		return
	}

	// Compare basic fields
	fields := []struct {
		name    string
		cached  interface{}
		fetched interface{}
	}{
		{"Id", cached.Id, fetched.Id},
		{"Name", cached.Name, fetched.Name},
		{"Description", cached.Description, fetched.Description},
		{"IsTopLevel", cached.IsTopLevel, fetched.IsTopLevel},
		{"IsHidden", cached.IsHidden, fetched.IsHidden},
		{"IsIntro", cached.IsIntro, fetched.IsIntro},
		{"IsUpgradeable", cached.IsUpgradeable, fetched.IsUpgradeable},
		{"ColorStr", cached.ColorStr, fetched.ColorStr},
		{"Offer", cached.Offer, fetched.Offer},
	}

	for _, field := range fields {
		if field.cached != field.fetched {
			log.Warn("Product field differs", zap.String("field", field.name), zap.Any("cached", field.cached), zap.Any("fetched", field.fetched))
		}
	}

	// Compare price arrays
	comparePriceArraysForDiff(log, "PricesYearly", cached.PricesYearly, fetched.PricesYearly)
	comparePriceArraysForDiff(log, "PricesMonthly", cached.PricesMonthly, fetched.PricesMonthly)

	// Compare features
	compareFeaturesForDiff(log, cached.Features, fetched.Features)
}

func comparePriceArraysForDiff(log *logging.Sugared, name string, cached []*model.MembershipV2Amount, fetched []*model.MembershipV2Amount) {
	cachedNil := cached == nil
	fetchedNil := fetched == nil
	cachedEmpty := len(cached) == 0
	fetchedEmpty := len(fetched) == 0

	if cachedNil && fetchedNil {
		log.Info("Both price arrays are nil", zap.String("array", name))
		return
	}
	if cachedNil && fetchedEmpty {
		log.Info("Price array nil vs empty", zap.String("array", name), zap.String("status", "cached nil, fetched empty"))
		return
	}
	if cachedEmpty && fetchedNil {
		log.Info("Price array nil vs empty", zap.String("array", name), zap.String("status", "cached empty, fetched nil"))
		return
	}
	if cachedNil || fetchedNil {
		log.Warn("Price array nil status differs", zap.String("array", name), zap.Bool("cachedNil", cachedNil), zap.Bool("fetchedNil", fetchedNil))
		return
	}

	if len(cached) != len(fetched) {
		log.Warn("Price array lengths differ", zap.String("array", name), zap.Int("cachedLen", len(cached)), zap.Int("fetchedLen", len(fetched)))
		return
	}

	for i := range cached {
		if cached[i] == nil && fetched[i] == nil {
			continue
		}
		if cached[i] == nil || fetched[i] == nil {
			log.Warn("Price array element nil status differs", zap.String("array", name), zap.Int("index", i), zap.Bool("cachedNil", cached[i] == nil), zap.Bool("fetchedNil", fetched[i] == nil))
			continue
		}
		if cached[i].Currency != fetched[i].Currency || cached[i].AmountCents != fetched[i].AmountCents {
			log.Warn("Price array element differs", zap.String("array", name), zap.Int("index", i), zap.String("cachedCurrency", cached[i].Currency), zap.Int64("cachedAmount", cached[i].AmountCents), zap.String("fetchedCurrency", fetched[i].Currency), zap.Int64("fetchedAmount", fetched[i].AmountCents))
		}
	}
}

func compareFeaturesForDiff(log *logging.Sugared, cached *model.MembershipV2Features, fetched *model.MembershipV2Features) {
	if cached == nil && fetched == nil {
		log.Info("Both features are nil")
		return
	}
	if cached == nil || fetched == nil {
		log.Warn("Features nil status differs", zap.Bool("cachedNil", cached == nil), zap.Bool("fetchedNil", fetched == nil))
		return
	}

	fields := []struct {
		name    string
		cached  interface{}
		fetched interface{}
	}{
		{"StorageBytes", cached.StorageBytes, fetched.StorageBytes},
		{"SpaceReaders", cached.SpaceReaders, fetched.SpaceReaders},
		{"SpaceWriters", cached.SpaceWriters, fetched.SpaceWriters},
		{"SharedSpaces", cached.SharedSpaces, fetched.SharedSpaces},
		{"TeamSeats", cached.TeamSeats, fetched.TeamSeats},
		{"AnyNameCount", cached.AnyNameCount, fetched.AnyNameCount},
		{"AnyNameMinLen", cached.AnyNameMinLen, fetched.AnyNameMinLen},
	}

	for _, field := range fields {
		if field.cached != field.fetched {
			log.Warn("Features field differs", zap.String("field", field.name), zap.Any("cached", field.cached), zap.Any("fetched", field.fetched))
		}
	}
}

func comparePurchaseInfo(log *logging.Sugared, cached *model.MembershipV2PurchaseInfo, fetched *model.MembershipV2PurchaseInfo) {
	if cached == nil && fetched == nil {
		log.Info("Both purchase info are nil")
		return
	}
	if cached == nil || fetched == nil {
		log.Warn("Purchase info nil status differs", zap.Bool("cachedNil", cached == nil), zap.Bool("fetchedNil", fetched == nil))
		return
	}

	fields := []struct {
		name    string
		cached  interface{}
		fetched interface{}
	}{
		{"DateStarted", cached.DateStarted, fetched.DateStarted},
		{"DateEnds", cached.DateEnds, fetched.DateEnds},
		{"IsAutoRenew", cached.IsAutoRenew, fetched.IsAutoRenew},
		{"Period", cached.Period, fetched.Period},
	}

	for _, field := range fields {
		if field.cached != field.fetched {
			log.Warn("Purchase info field differs", zap.String("field", field.name), zap.Any("cached", field.cached), zap.Any("fetched", field.fetched))
		}
	}
}

func compareProductStatus(log *logging.Sugared, cached *model.MembershipV2ProductStatus, fetched *model.MembershipV2ProductStatus) {
	if cached == nil && fetched == nil {
		log.Info("Both product status are nil")
		return
	}
	if cached == nil || fetched == nil {
		log.Warn("Product status nil status differs", zap.Bool("cachedNil", cached == nil), zap.Bool("fetchedNil", fetched == nil))
		return
	}

	if cached.Status != fetched.Status {
		log.Warn("Product status differs", zap.Int32("cached", int32(cached.Status)), zap.Int32("fetched", int32(fetched.Status)))
	}
}

func compareNextInvoice(log *logging.Sugared, cached *model.MembershipV2Invoice, fetched *model.MembershipV2Invoice) {
	if cached == nil && fetched == nil {
		log.Info("Both next invoices are nil")
		return
	}
	if cached == nil || fetched == nil {
		log.Warn("Next invoice nil status differs", zap.Bool("cachedNil", cached == nil), zap.Bool("fetchedNil", fetched == nil))
		return
	}

	if cached.Date != fetched.Date {
		log.Warn("Next invoice date differs", zap.Uint64("cached", cached.Date), zap.Uint64("fetched", fetched.Date))
	} else {
		log.Info("Next invoice date matches", zap.Uint64("value", cached.Date))
	}

	if (cached.Total == nil) != (fetched.Total == nil) {
		log.Warn("Next invoice total nil status differs", zap.Bool("cachedNil", cached.Total == nil), zap.Bool("fetchedNil", fetched.Total == nil))
		return
	}

	if cached.Total != nil && fetched.Total != nil {
		if cached.Total.Currency != fetched.Total.Currency || cached.Total.AmountCents != fetched.Total.AmountCents {
			log.Warn("Next invoice total differs", zap.String("cachedCurrency", cached.Total.Currency), zap.Int64("cachedAmount", cached.Total.AmountCents), zap.String("fetchedCurrency", fetched.Total.Currency), zap.Int64("fetchedAmount", fetched.Total.AmountCents))
		} else {
			log.Info("Next invoice total matches", zap.String("currency", cached.Total.Currency), zap.Int64("amount", cached.Total.AmountCents))
		}
	}
}

func logV2ProductsDiff(cached []*model.MembershipV2Product, fetched []*model.MembershipV2Product) {
	log := logging.Logger("payments-products-diff")

	log.Info("=== V2 Products Diff Analysis ===")

	// Handle nil cases
	if cached == nil && fetched == nil {
		log.Info("Both product slices are nil")
		return
	}
	if cached == nil || fetched == nil {
		log.Warn("One product slice is nil", zap.Bool("cachedNil", cached == nil), zap.Bool("fetchedNil", fetched == nil))
		return
	}

	// Compare slices
	compareProductSlices(log, cached, fetched)

	log.Info("=== End V2 Products Diff Analysis ===")
}

func compareProductSlices(log *logging.Sugared, cached []*model.MembershipV2Product, fetched []*model.MembershipV2Product) {
	// Handle nil/empty cases
	cachedNil := cached == nil
	fetchedNil := fetched == nil
	cachedEmpty := len(cached) == 0
	fetchedEmpty := len(fetched) == 0

	if cachedNil && fetchedNil {
		log.Info("Both product slices are nil")
		return
	}
	if cachedNil && fetchedEmpty {
		log.Info("Cached products nil, fetched empty")
		return
	}
	if cachedEmpty && fetchedNil {
		log.Info("Cached products empty, fetched nil")
		return
	}
	if cachedNil || fetchedNil {
		log.Warn("Product slice nil status differs", zap.Bool("cachedNil", cachedNil), zap.Bool("fetchedNil", fetchedNil))
		return
	}

	if len(cached) != len(fetched) {
		log.Warn("Product slice lengths differ", zap.Int("cachedLen", len(cached)), zap.Int("fetchedLen", len(fetched)))
		return
	}

	log.Info("Comparing products", zap.Int("count", len(cached)))
	for i := range cached {
		log.Info("--- Product comparison ---", zap.Int("index", i))
		compareProduct(log, cached[i], fetched[i])
	}
}
