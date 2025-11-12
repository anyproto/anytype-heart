package payments

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	paymentserviceproto "github.com/anyproto/any-sync/paymentservice/paymentserviceproto"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/nameservice"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestTierDataFieldParity(t *testing.T) {
	compareStructFields(t,
		reflect.TypeOf(paymentserviceproto.TierData{}),
		reflect.TypeOf(model.MembershipTierData{}),
		nil,
		[]string{"IsActive", "IsHiddenTier"},
	)
}

func TestMembershipFieldParity(t *testing.T) {
	compareStructFields(t,
		reflect.TypeOf(paymentserviceproto.GetSubscriptionResponse{}),
		reflect.TypeOf(model.Membership{}),
		map[string]string{
			"NsName":     "RequestedAnyName",
			"NsNameType": "RequestedAnyName",
		},
		nil,
	)
}

func TestConvertTierData_JSONCoverage(t *testing.T) {
	src := &paymentserviceproto.TierData{
		Id:                    42,
		Name:                  "Pro plan",
		Description:           "Premium access tier",
		IsActive:              true,
		IsTest:                true,
		IsHiddenTier:          true,
		PeriodType:            paymentserviceproto.PeriodType_PeriodTypeMonths,
		PeriodValue:           12,
		PriceStripeUsdCents:   1999,
		AnyNamesCountIncluded: 3,
		AnyNameMinLength:      5,
		Features: []*paymentserviceproto.Feature{
			{Description: "Feature A"},
			{Description: "Feature B"},
		},
		ColorStr:         "#ff00ff",
		StripeProductId:  "stripe-prod-id",
		StripeManageUrl:  "https://stripe.example/manage",
		IosProductId:     "ios-prod-id",
		IosManageUrl:     "https://ios.example/manage",
		AndroidProductId: "android-prod-id",
		AndroidManageUrl: "https://android.example/manage",
		Offer:            "Launch offer",
	}

	actual := convertTierData(src)
	require.NotNil(t, actual)

	expected := &model.MembershipTierData{
		Id:                    src.Id,
		Name:                  src.Name,
		Description:           src.Description,
		IsTest:                src.IsTest,
		PeriodType:            model.MembershipTierDataPeriodType(src.PeriodType),
		PeriodValue:           src.PeriodValue,
		PriceStripeUsdCents:   src.PriceStripeUsdCents,
		AnyNamesCountIncluded: src.AnyNamesCountIncluded,
		AnyNameMinLength:      src.AnyNameMinLength,
		Features: []string{
			src.Features[0].Description,
			src.Features[1].Description,
		},
		ColorStr:         src.ColorStr,
		StripeProductId:  src.StripeProductId,
		StripeManageUrl:  src.StripeManageUrl,
		IosProductId:     src.IosProductId,
		IosManageUrl:     src.IosManageUrl,
		AndroidProductId: src.AndroidProductId,
		AndroidManageUrl: src.AndroidManageUrl,
		Offer:            src.Offer,
	}

	require.Equal(t, expected, actual)

	assertAllExportedFieldsNonZeroAndInJSON(t, actual, nil)
}

func TestConvertMembershipData_JSONCoverage(t *testing.T) {
	src := &paymentserviceproto.GetSubscriptionResponse{
		Tier:                  7,
		Status:                paymentserviceproto.SubscriptionStatus_StatusActive,
		DateStarted:           1_694_196_800,
		DateEnds:              1_727_750_400,
		IsAutoRenew:           true,
		PaymentMethod:         paymentserviceproto.PaymentMethod_MethodGoogleInapp,
		RequestedAnyName:      "member-name.any",
		UserEmail:             "member@example.com",
		SubscribeToNewsletter: true,
	}

	actual := convertMembershipData(src)
	require.NotNil(t, actual)

	expectedNsName, expectedNsNameType := nameservice.FullNameToNsName(src.RequestedAnyName)

	expected := &model.Membership{
		Tier:                  src.Tier,
		Status:                model.MembershipStatus(src.Status),
		DateStarted:           src.DateStarted,
		DateEnds:              src.DateEnds,
		IsAutoRenew:           src.IsAutoRenew,
		PaymentMethod:         PaymentMethodToModel(src.PaymentMethod),
		NsName:                expectedNsName,
		NsNameType:            expectedNsNameType,
		UserEmail:             src.UserEmail,
		SubscribeToNewsletter: src.SubscribeToNewsletter,
	}

	require.Equal(t, expected, actual)

	assertAllExportedFieldsNonZeroAndInJSON(t, actual, map[string]struct{}{
		"NsNameType": {},
	})
}

func assertAllExportedFieldsNonZeroAndInJSON(t *testing.T, val any, allowedZero map[string]struct{}) {
	t.Helper()

	raw, err := json.Marshal(val)
	require.NoError(t, err)

	var asJSON map[string]any
	require.NoError(t, json.Unmarshal(raw, &asJSON))

	value := reflect.ValueOf(val)
	if value.Kind() == reflect.Ptr {
		require.False(t, value.IsNil(), "value cannot be nil")
		value = value.Elem()
	}

	typ := value.Type()
	fields := exportedFieldSet(typ)

	for fieldName := range fields {
		fieldValue := value.FieldByName(fieldName)
		structField, ok := typ.FieldByName(fieldName)
		require.Truef(t, ok, "field %s metadata missing", fieldName)

		isAllowedZero := containsField(allowedZero, fieldName)

		if !isAllowedZero {
			require.Falsef(t, fieldValue.IsZero(), "field %s should not be zero", fieldName)
		} else {
			// Skip JSON assertion for fields explicitly allowed to be zero (e.g. omitempty tags).
			continue
		}

		jsonKey := structField.Tag.Get("json")
		if jsonKey == "" {
			jsonKey = structField.Name
		} else {
			jsonKey = strings.Split(jsonKey, ",")[0]
		}
		if jsonKey == "-" || jsonKey == "" {
			continue
		}

		_, found := asJSON[jsonKey]
		require.Truef(t, found, "json output missing key %s for field %s", jsonKey, fieldName)
	}
}

func containsField(set map[string]struct{}, name string) bool {
	if set == nil {
		return false
	}
	_, ok := set[name]
	return ok
}

func compareStructFields(t *testing.T, srcType, dstType reflect.Type, dstToSrc map[string]string, allowedSrcExtras []string) {
	t.Helper()

	srcFields := exportedFieldSet(srcType)
	dstFields := exportedFieldSet(dstType)

	if dstToSrc == nil {
		dstToSrc = make(map[string]string)
	}

	// Check that each destination field has a matching source field (direct name or via mapping).
	var missingInSrc []string
	matchedSrc := make(map[string]struct{})
	for dstField := range dstFields {
		srcField := dstField
		if mapped, ok := dstToSrc[dstField]; ok {
			srcField = mapped
		}
		if _, ok := srcFields[srcField]; !ok {
			missingInSrc = append(missingInSrc, dstField)
			continue
		}
		matchedSrc[srcField] = struct{}{}
	}
	require.Emptyf(t, missingInSrc, "destination-only fields detected: %v", missingInSrc)

	// Identify source fields that are not accounted for by destination struct.
	extrasInSrc := difference(srcFields, matchedSrc)

	allowed := make(map[string]struct{}, len(allowedSrcExtras))
	for _, name := range allowedSrcExtras {
		allowed[name] = struct{}{}
	}

	var unexpectedExtras []string
	for extra := range extrasInSrc {
		if _, ok := allowed[extra]; ok {
			continue
		}
		unexpectedExtras = append(unexpectedExtras, extra)
	}
	require.Emptyf(t, unexpectedExtras, "unexpected source-only fields detected: %v", unexpectedExtras)
}

func exportedFieldSet(structType reflect.Type) map[string]struct{} {
	result := make(map[string]struct{})
	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		if field.PkgPath != "" {
			continue
		}
		result[field.Name] = struct{}{}
	}
	return result
}

func difference(fullSet map[string]struct{}, subset map[string]struct{}) map[string]struct{} {
	result := make(map[string]struct{})
	for name := range fullSet {
		if _, ok := subset[name]; ok {
			continue
		}
		result[name] = struct{}{}
	}
	return result
}

func TestProductFieldParity(t *testing.T) {
	compareStructFields(t,
		reflect.TypeOf(paymentserviceproto.MembershipV2_Product{}),
		reflect.TypeOf(model.MembershipV2Product{}),
		nil,
		nil,
	)
}

func TestConvertProductData_JSONCoverage(t *testing.T) {
	src := &paymentserviceproto.MembershipV2_Product{
		Id:            "prod_123",
		Name:          "Plus",
		Description:   "Best value",
		IsTopLevel:    true,
		IsHidden:      true,
		IsIntro:       true,
		IsUpgradeable: true,
		PricesYearly:  []*paymentserviceproto.MembershipV2_Amount{{Currency: "USD", AmountCents: 4800}, {Currency: "EUR", AmountCents: 4500}},
		PricesMonthly: []*paymentserviceproto.MembershipV2_Amount{{Currency: "USD", AmountCents: 500}, {Currency: "EUR", AmountCents: 450}},
		ColorStr:      "blue",
		Offer:         "intro",
		Features: &paymentserviceproto.MembershipV2_Features{
			StorageBytes:  100 * 1024 * 1024,
			SpaceReaders:  10,
			SpaceWriters:  5,
			SharedSpaces:  20,
			TeamSeats:     3,
			AnyNameCount:  1,
			AnyNameMinLen: 9,
		},
	}

	actual := convertProductData(src)
	require.NotNil(t, actual)

	expected := &model.MembershipV2Product{
		Id:            src.Id,
		Name:          src.Name,
		Description:   src.Description,
		IsTopLevel:    src.IsTopLevel,
		IsHidden:      src.IsHidden,
		IsIntro:       src.IsIntro,
		IsUpgradeable: src.IsUpgradeable,
		ColorStr:      src.ColorStr,
		Offer:         src.Offer,
		PricesYearly: []*model.MembershipV2Amount{
			{Currency: src.PricesYearly[0].Currency, AmountCents: src.PricesYearly[0].AmountCents},
			{Currency: src.PricesYearly[1].Currency, AmountCents: src.PricesYearly[1].AmountCents},
		},
		PricesMonthly: []*model.MembershipV2Amount{
			{Currency: src.PricesMonthly[0].Currency, AmountCents: src.PricesMonthly[0].AmountCents},
			{Currency: src.PricesMonthly[1].Currency, AmountCents: src.PricesMonthly[1].AmountCents},
		},
		Features: &model.MembershipV2Features{
			StorageBytes:  src.Features.StorageBytes,
			SpaceReaders:  src.Features.SpaceReaders,
			SpaceWriters:  src.Features.SpaceWriters,
			SharedSpaces:  src.Features.SharedSpaces,
			TeamSeats:     src.Features.TeamSeats,
			AnyNameCount:  src.Features.AnyNameCount,
			AnyNameMinLen: src.Features.AnyNameMinLen,
		},
	}

	require.Equal(t, expected, actual)

	assertAllExportedFieldsNonZeroAndInJSON(t, actual, nil)
}

func TestPurchasedProductFieldParity(t *testing.T) {
	compareStructFields(t,
		reflect.TypeOf(paymentserviceproto.MembershipV2_PurchasedProduct{}),
		reflect.TypeOf(model.MembershipV2PurchasedProduct{}),
		nil,
		nil,
	)
}

func TestConvertPurchasedProductData_JSONCoverage(t *testing.T) {
	src := &paymentserviceproto.MembershipV2_PurchasedProduct{
		Product: &paymentserviceproto.MembershipV2_Product{
			Id:           "prod_123",
			Name:         "Plus",
			Description:  "Best value",
			IsTopLevel:   true,
			IsHidden:     true,
			PricesYearly: []*paymentserviceproto.MembershipV2_Amount{{Currency: "USD", AmountCents: 4800}},
			PricesMonthly: []*paymentserviceproto.MembershipV2_Amount{
				{Currency: "USD", AmountCents: 500},
			},
			ColorStr: "blue",
			Offer:    "intro",
			Features: &paymentserviceproto.MembershipV2_Features{
				StorageBytes:  100 * 1024 * 1024,
				SpaceReaders:  10,
				SpaceWriters:  5,
				SharedSpaces:  20,
				TeamSeats:     3,
				AnyNameCount:  1,
				AnyNameMinLen: 9,
			},
		},
		PurchaseInfo: &paymentserviceproto.MembershipV2_PurchaseInfo{
			DateStarted: 1_700_000_000,
			DateEnds:    1_800_000_000,
			IsAutoRenew: true,
			Period:      paymentserviceproto.MembershipV2_Monthly,
		},
		ProductStatus: &paymentserviceproto.MembershipV2_ProductStatus{
			Status: paymentserviceproto.MembershipV2_ProductStatus_StatusActive,
		},
	}

	actual := convertPurchasedProductData(src)
	require.NotNil(t, actual)

	expected := &model.MembershipV2PurchasedProduct{
		Product: &model.MembershipV2Product{
			Id:          src.Product.Id,
			Name:        src.Product.Name,
			Description: src.Product.Description,
			IsTopLevel:  src.Product.IsTopLevel,
			IsHidden:    src.Product.IsHidden,
			ColorStr:    src.Product.ColorStr,
			Offer:       src.Product.Offer,
			PricesYearly: []*model.MembershipV2Amount{
				{Currency: src.Product.PricesYearly[0].Currency, AmountCents: src.Product.PricesYearly[0].AmountCents},
			},
			PricesMonthly: []*model.MembershipV2Amount{
				{Currency: src.Product.PricesMonthly[0].Currency, AmountCents: src.Product.PricesMonthly[0].AmountCents},
			},
			Features: &model.MembershipV2Features{
				StorageBytes:  src.Product.Features.StorageBytes,
				SpaceReaders:  src.Product.Features.SpaceReaders,
				SpaceWriters:  src.Product.Features.SpaceWriters,
				SharedSpaces:  src.Product.Features.SharedSpaces,
				TeamSeats:     src.Product.Features.TeamSeats,
				AnyNameCount:  src.Product.Features.AnyNameCount,
				AnyNameMinLen: src.Product.Features.AnyNameMinLen,
			},
		},
		PurchaseInfo: &model.MembershipV2PurchaseInfo{
			DateStarted: src.PurchaseInfo.DateStarted,
			DateEnds:    src.PurchaseInfo.DateEnds,
			IsAutoRenew: src.PurchaseInfo.IsAutoRenew,
			Period:      model.MembershipV2Period(src.PurchaseInfo.Period),
		},
		ProductStatus: &model.MembershipV2ProductStatus{
			Status: model.MembershipV2ProductStatusStatus(src.ProductStatus.Status),
		},
	}

	require.Equal(t, expected, actual)
	assertAllExportedFieldsNonZeroAndInJSON(t, actual, nil)
}

/*
func TestInvoiceFieldParity(t *testing.T) {
	// NOTE: this should fail due to [Status Id] mismatch
	// (as expected)
	compareStructFields(t,
		reflect.TypeOf(paymentserviceproto.MembershipV2_Invoice{}),
		reflect.TypeOf(model.MembershipV2Invoice{}),
		nil,
		nil,
	)
}
*/

func TestConvertInvoiceData_JSONCoverage(t *testing.T) {
	src := &paymentserviceproto.MembershipV2_Invoice{
		Date:  1_750_000_000,
		Total: &paymentserviceproto.MembershipV2_Amount{Currency: "USD", AmountCents: 9600},
	}

	actual := convertInvoiceData(src)
	require.NotNil(t, actual)

	expected := &model.MembershipV2Invoice{
		Date: src.Date,
		Total: &model.MembershipV2Amount{
			Currency:    src.Total.Currency,
			AmountCents: src.Total.AmountCents,
		},
	}

	require.Equal(t, expected, actual)
	assertAllExportedFieldsNonZeroAndInJSON(t, actual, nil)
}

func TestCartProductFieldParity(t *testing.T) {
	compareStructFields(t,
		reflect.TypeOf(paymentserviceproto.MembershipV2_CartProduct{}),
		reflect.TypeOf(model.MembershipV2CartProduct{}),
		nil,
		nil,
	)
}

func TestConvertCartProductData_JSONCoverage(t *testing.T) {
	src := &paymentserviceproto.MembershipV2_CartProduct{
		Product: &paymentserviceproto.MembershipV2_Product{
			Id:            "prod_123",
			Name:          "Plus",
			Description:   "Best value",
			IsTopLevel:    true,
			IsHidden:      true,
			IsIntro:       true,
			IsUpgradeable: true,
			PricesYearly:  []*paymentserviceproto.MembershipV2_Amount{{Currency: "USD", AmountCents: 4800}},
			PricesMonthly: []*paymentserviceproto.MembershipV2_Amount{
				{Currency: "USD", AmountCents: 500},
			},
			ColorStr: "blue",
			Offer:    "intro",
			Features: &paymentserviceproto.MembershipV2_Features{
				StorageBytes:  100 * 1024 * 1024,
				SpaceReaders:  10,
				SpaceWriters:  5,
				SharedSpaces:  20,
				TeamSeats:     3,
				AnyNameCount:  1,
				AnyNameMinLen: 9,
			},
		},
		IsYearly: true,
		Remove:   true,
	}

	actual := convertCartProductData(src)
	require.NotNil(t, actual)

	expected := &model.MembershipV2CartProduct{
		Product: &model.MembershipV2Product{
			Id:            src.Product.Id,
			Name:          src.Product.Name,
			Description:   src.Product.Description,
			IsTopLevel:    src.Product.IsTopLevel,
			IsHidden:      src.Product.IsHidden,
			IsIntro:       src.Product.IsIntro,
			IsUpgradeable: src.Product.IsUpgradeable,
			ColorStr:      src.Product.ColorStr,
			Offer:         src.Product.Offer,
			PricesYearly: []*model.MembershipV2Amount{
				{Currency: src.Product.PricesYearly[0].Currency, AmountCents: src.Product.PricesYearly[0].AmountCents},
			},
			PricesMonthly: []*model.MembershipV2Amount{
				{Currency: src.Product.PricesMonthly[0].Currency, AmountCents: src.Product.PricesMonthly[0].AmountCents},
			},
			Features: &model.MembershipV2Features{
				StorageBytes:  src.Product.Features.StorageBytes,
				SpaceReaders:  src.Product.Features.SpaceReaders,
				SpaceWriters:  src.Product.Features.SpaceWriters,
				SharedSpaces:  src.Product.Features.SharedSpaces,
				TeamSeats:     src.Product.Features.TeamSeats,
				AnyNameCount:  src.Product.Features.AnyNameCount,
				AnyNameMinLen: src.Product.Features.AnyNameMinLen,
			},
		},
		IsYearly: src.IsYearly,
		Remove:   src.Remove,
	}

	require.Equal(t, expected, actual)
	assertAllExportedFieldsNonZeroAndInJSON(t, actual, nil)
}

func TestCartFieldParity(t *testing.T) {
	// Compare Cart struct (not the full response wrapper)
	compareStructFields(t,
		reflect.TypeOf(paymentserviceproto.MembershipV2_Cart{}),
		reflect.TypeOf(model.MembershipV2Cart{}),
		nil,
		nil,
	)
}

func TestTiersAreEqual(t *testing.T) {
	// Test nil cases
	require.True(t, tiersAreEqual(nil, nil))
	require.True(t, tiersAreEqual(nil, []*model.MembershipTierData{}))
	require.True(t, tiersAreEqual([]*model.MembershipTierData{}, nil))

	// Test empty slices
	empty1 := []*model.MembershipTierData{}
	empty2 := []*model.MembershipTierData{}
	require.True(t, tiersAreEqual(empty1, empty2), "Empty slices should be equal")

	// Test identical tiers
	tier1 := &model.MembershipTierData{
		Id:                    1,
		Name:                  "Basic Plan",
		Description:           "Basic tier",
		IsTest:                false,
		PeriodType:            model.MembershipTierData_PeriodTypeMonths,
		PeriodValue:           1,
		PriceStripeUsdCents:   500,
		AnyNamesCountIncluded: 1,
		AnyNameMinLength:      7,
		Features:              []string{"Feature 1", "Feature 2"},
		ColorStr:              "blue",
		StripeProductId:       "prod_basic",
		StripeManageUrl:       "https://stripe.com/manage",
		IosProductId:          "ios_basic",
		IosManageUrl:          "https://ios.com/manage",
		AndroidProductId:      "android_basic",
		AndroidManageUrl:      "https://android.com/manage",
		Offer:                 "basic_offer",
	}

	tier2 := &model.MembershipTierData{
		Id:                    2,
		Name:                  "Pro Plan",
		Description:           "Professional tier",
		IsTest:                false,
		PeriodType:            model.MembershipTierData_PeriodTypeYears,
		PeriodValue:           1,
		PriceStripeUsdCents:   5000,
		AnyNamesCountIncluded: 5,
		AnyNameMinLength:      5,
		Features:              []string{"Pro Feature 1", "Pro Feature 2", "Pro Feature 3"},
		ColorStr:              "gold",
		StripeProductId:       "prod_pro",
		StripeManageUrl:       "https://stripe.com/manage_pro",
		IosProductId:          "ios_pro",
		IosManageUrl:          "https://ios.com/manage_pro",
		AndroidProductId:      "android_pro",
		AndroidManageUrl:      "https://android.com/manage_pro",
		Offer:                 "pro_offer",
	}

	// Test identical single tier
	tiers1 := []*model.MembershipTierData{tier1}
	tiers2 := []*model.MembershipTierData{tier1}
	require.True(t, tiersAreEqual(tiers1, tiers2), "Identical single tiers should be equal")

	// Test identical multiple tiers
	tiers3 := []*model.MembershipTierData{tier1, tier2}
	tiers4 := []*model.MembershipTierData{tier1, tier2}
	require.True(t, tiersAreEqual(tiers3, tiers4), "Identical multiple tiers should be equal")

	// Test different tiers
	tiers5 := []*model.MembershipTierData{tier1}
	tiers6 := []*model.MembershipTierData{tier2}
	require.False(t, tiersAreEqual(tiers5, tiers6), "Different tiers should not be equal")

	// Test different lengths
	tiers7 := []*model.MembershipTierData{tier1}
	tiers8 := []*model.MembershipTierData{tier1, tier2}
	require.False(t, tiersAreEqual(tiers7, tiers8), "Tiers with different lengths should not be equal")

	// Test nil vs empty features within tier (this may fail with current implementation)
	tierWithNilFeatures := &model.MembershipTierData{
		Id:                    1,
		Name:                  "Basic Plan",
		Description:           "Basic tier",
		IsTest:                false,
		PeriodType:            model.MembershipTierData_PeriodTypeMonths,
		PeriodValue:           1,
		PriceStripeUsdCents:   500,
		AnyNamesCountIncluded: 1,
		AnyNameMinLength:      7,
		Features:              nil, // nil instead of empty slice
		ColorStr:              "blue",
		StripeProductId:       "prod_basic",
		StripeManageUrl:       "https://stripe.com/manage",
		IosProductId:          "ios_basic",
		IosManageUrl:          "https://ios.com/manage",
		AndroidProductId:      "android_basic",
		AndroidManageUrl:      "https://android.com/manage",
		Offer:                 "basic_offer",
	}

	tierWithEmptyFeatures := &model.MembershipTierData{
		Id:                    1,
		Name:                  "Basic Plan",
		Description:           "Basic tier",
		IsTest:                false,
		PeriodType:            model.MembershipTierData_PeriodTypeMonths,
		PeriodValue:           1,
		PriceStripeUsdCents:   500,
		AnyNamesCountIncluded: 1,
		AnyNameMinLength:      7,
		Features:              []string{}, // empty slice instead of nil
		ColorStr:              "blue",
		StripeProductId:       "prod_basic",
		StripeManageUrl:       "https://stripe.com/manage",
		IosProductId:          "ios_basic",
		IosManageUrl:          "https://ios.com/manage",
		AndroidProductId:      "android_basic",
		AndroidManageUrl:      "https://android.com/manage",
		Offer:                 "basic_offer",
	}

	tiersWithNil := []*model.MembershipTierData{tierWithNilFeatures}
	tiersWithEmpty := []*model.MembershipTierData{tierWithEmptyFeatures}

	// This test may fail because deriveEqual_1 doesn't treat nil and empty slices as equal
	result := tiersAreEqual(tiersWithNil, tiersWithEmpty)
	t.Logf("Tiers with nil vs empty features equal: %v", result)

	// WARNING: yes, nil and empty slices ARE NOT EQUAL!
	require.False(t, result, "Tiers with nil vs empty features should be equal")

	// Test with nil tier in slice
	tiersWithNilTier := []*model.MembershipTierData{nil}
	tiersWithNilTier2 := []*model.MembershipTierData{nil}
	require.True(t, tiersAreEqual(tiersWithNilTier, tiersWithNilTier2), "Slices with nil tiers should be equal")

	// Test mixed nil and non-nil
	tiersMixed1 := []*model.MembershipTierData{nil, tier1}
	tiersMixed2 := []*model.MembershipTierData{nil, tier1}
	require.True(t, tiersAreEqual(tiersMixed1, tiersMixed2), "Mixed nil/non-nil tiers should be equal when identical")

	tiersMixed3 := []*model.MembershipTierData{tier1, nil}
	require.False(t, tiersAreEqual(tiersMixed1, tiersMixed3), "Different nil positions should not be equal")
}

func TestProductsV2Equal(t *testing.T) {
	// Test nil cases
	require.True(t, productsV2Equal(nil, nil))
	require.True(t, productsV2Equal(nil, []*model.MembershipV2Product{}))
	require.True(t, productsV2Equal([]*model.MembershipV2Product{}, nil))

	// Test empty slices
	empty1 := []*model.MembershipV2Product{}
	empty2 := []*model.MembershipV2Product{}
	require.True(t, productsV2Equal(empty1, empty2), "Empty slices should be equal")

	// Test identical products
	product1 := &model.MembershipV2Product{
		Id:            "prod_123",
		Name:          "Test Product",
		Description:   "Test Description",
		IsTopLevel:    true,
		IsHidden:      false,
		IsIntro:       false,
		IsUpgradeable: true,
		PricesYearly: []*model.MembershipV2Amount{
			{Currency: "USD", AmountCents: 1000},
		},
		PricesMonthly: []*model.MembershipV2Amount{
			{Currency: "USD", AmountCents: 100},
		},
		ColorStr: "blue",
		Offer:    "test_offer",
		Features: &model.MembershipV2Features{
			StorageBytes:  1000000,
			SpaceReaders:  5,
			SpaceWriters:  2,
			SharedSpaces:  3,
			TeamSeats:     10,
			AnyNameCount:  2,
			AnyNameMinLen: 3,
		},
	}

	product2 := &model.MembershipV2Product{
		Id:            "prod_456",
		Name:          "Another Product",
		Description:   "Another Description",
		IsTopLevel:    false,
		IsHidden:      true,
		IsIntro:       true,
		IsUpgradeable: false,
		PricesYearly: []*model.MembershipV2Amount{
			{Currency: "EUR", AmountCents: 900},
		},
		PricesMonthly: []*model.MembershipV2Amount{
			{Currency: "EUR", AmountCents: 90},
		},
		ColorStr: "red",
		Offer:    "another_offer",
		Features: &model.MembershipV2Features{
			StorageBytes:  2000000,
			SpaceReaders:  10,
			SpaceWriters:  5,
			SharedSpaces:  5,
			TeamSeats:     20,
			AnyNameCount:  5,
			AnyNameMinLen: 5,
		},
	}

	// Test identical single product
	products1 := []*model.MembershipV2Product{product1}
	products2 := []*model.MembershipV2Product{product1}
	require.True(t, productsV2Equal(products1, products2), "Identical single products should be equal")

	// Test identical multiple products
	products3 := []*model.MembershipV2Product{product1, product2}
	products4 := []*model.MembershipV2Product{product1, product2}
	require.True(t, productsV2Equal(products3, products4), "Identical multiple products should be equal")

	// Test different products
	products5 := []*model.MembershipV2Product{product1}
	products6 := []*model.MembershipV2Product{product2}
	require.False(t, productsV2Equal(products5, products6), "Different products should not be equal")

	// Test different lengths
	products7 := []*model.MembershipV2Product{product1}
	products8 := []*model.MembershipV2Product{product1, product2}
	require.False(t, productsV2Equal(products7, products8), "Products with different lengths should not be equal")

	// Test nil vs empty slice within product prices (this should be handled by the Equal method)
	productWithNilPrices := &model.MembershipV2Product{
		Id:            "prod_123",
		Name:          "Test Product",
		Description:   "Test Description",
		IsTopLevel:    true,
		IsHidden:      false,
		IsIntro:       false,
		IsUpgradeable: true,
		PricesYearly:  nil, // nil instead of empty slice
		PricesMonthly: nil, // nil instead of empty slice
		ColorStr:      "blue",
		Offer:         "test_offer",
		Features: &model.MembershipV2Features{
			StorageBytes:  1000000,
			SpaceReaders:  5,
			SpaceWriters:  2,
			SharedSpaces:  3,
			TeamSeats:     10,
			AnyNameCount:  2,
			AnyNameMinLen: 3,
		},
	}

	productWithEmptyPrices := &model.MembershipV2Product{
		Id:            "prod_123",
		Name:          "Test Product",
		Description:   "Test Description",
		IsTopLevel:    true,
		IsHidden:      false,
		IsIntro:       false,
		IsUpgradeable: true,
		PricesYearly:  []*model.MembershipV2Amount{}, // empty slice instead of nil
		PricesMonthly: []*model.MembershipV2Amount{}, // empty slice instead of nil
		ColorStr:      "blue",
		Offer:         "test_offer",
		Features: &model.MembershipV2Features{
			StorageBytes:  1000000,
			SpaceReaders:  5,
			SpaceWriters:  2,
			SharedSpaces:  3,
			TeamSeats:     10,
			AnyNameCount:  2,
			AnyNameMinLen: 3,
		},
	}

	productsWithNil := []*model.MembershipV2Product{productWithNilPrices}
	productsWithEmpty := []*model.MembershipV2Product{productWithEmptyPrices}
	// WARNING: yes, nil and empty slices ARE NOT EQUAL!
	require.False(t, productsV2Equal(productsWithNil, productsWithEmpty), "Products with nil vs empty price slices should not be equal (current implementation)")

	// Test with nil product in slice
	productsWithNilProduct := []*model.MembershipV2Product{nil}
	productsWithNilProduct2 := []*model.MembershipV2Product{nil}
	require.True(t, productsV2Equal(productsWithNilProduct, productsWithNilProduct2), "Slices with nil products should be equal")

	// Test mixed nil and non-nil
	productsMixed1 := []*model.MembershipV2Product{nil, product1}
	productsMixed2 := []*model.MembershipV2Product{nil, product1}
	require.True(t, productsV2Equal(productsMixed1, productsMixed2), "Mixed nil/non-nil products should be equal when identical")

	productsMixed3 := []*model.MembershipV2Product{product1, nil}
	require.False(t, productsV2Equal(productsMixed1, productsMixed3), "Different nil positions should not be equal")
}

func TestMembershipV2DataEqual(t *testing.T) {
	// Test nil cases
	require.True(t, membershipV2DataEqual(nil, nil))
	require.False(t, membershipV2DataEqual(nil, &model.MembershipV2Data{}))
	require.False(t, membershipV2DataEqual(&model.MembershipV2Data{}, nil))

	// Test empty objects
	empty1 := &model.MembershipV2Data{
		Products:        []*model.MembershipV2PurchasedProduct{},
		NextInvoice:     nil,
		TeamOwnerID:     "",
		PaymentProvider: 0,
	}
	empty2 := &model.MembershipV2Data{
		Products:        []*model.MembershipV2PurchasedProduct{},
		NextInvoice:     nil,
		TeamOwnerID:     "",
		PaymentProvider: 0,
	}
	require.True(t, membershipV2DataEqual(empty1, empty2), "Empty objects should be equal")

	// Test identical objects with data
	product := &model.MembershipV2PurchasedProduct{
		Product: &model.MembershipV2Product{
			Id:            "prod_123",
			Name:          "Test Product",
			Description:   "Test Description",
			IsTopLevel:    true,
			IsHidden:      false,
			IsIntro:       false,
			IsUpgradeable: true,
			PricesYearly: []*model.MembershipV2Amount{
				{Currency: "USD", AmountCents: 1000},
			},
			PricesMonthly: []*model.MembershipV2Amount{
				{Currency: "USD", AmountCents: 100},
			},
			ColorStr: "blue",
			Offer:    "test_offer",
			Features: &model.MembershipV2Features{
				StorageBytes:  1000000,
				SpaceReaders:  5,
				SpaceWriters:  2,
				SharedSpaces:  3,
				TeamSeats:     10,
				AnyNameCount:  2,
				AnyNameMinLen: 3,
			},
		},
		PurchaseInfo: &model.MembershipV2PurchaseInfo{
			DateStarted: 1234567890,
			DateEnds:    1234567891,
			IsAutoRenew: true,
			Period:      1,
		},
		ProductStatus: &model.MembershipV2ProductStatus{
			Status: 1, // Active
		},
	}

	// Test edge case: nil vs empty slices
	productWithNilSlices := &model.MembershipV2PurchasedProduct{
		Product: &model.MembershipV2Product{
			Id:            "prod_123",
			Name:          "Test Product",
			Description:   "Test Description",
			IsTopLevel:    true,
			IsHidden:      false,
			IsIntro:       false,
			IsUpgradeable: true,
			PricesYearly:  nil, // nil instead of empty slice
			PricesMonthly: nil, // nil instead of empty slice
			ColorStr:      "blue",
			Offer:         "test_offer",
			Features: &model.MembershipV2Features{
				StorageBytes:  1000000,
				SpaceReaders:  5,
				SpaceWriters:  2,
				SharedSpaces:  3,
				TeamSeats:     10,
				AnyNameCount:  2,
				AnyNameMinLen: 3,
			},
		},
		PurchaseInfo: &model.MembershipV2PurchaseInfo{
			DateStarted: 1234567890,
			DateEnds:    1234567891,
			IsAutoRenew: true,
			Period:      1,
		},
		ProductStatus: &model.MembershipV2ProductStatus{
			Status: 1, // Active
		},
	}

	invoice := &model.MembershipV2Invoice{
		Date: 1234567890,
		Total: &model.MembershipV2Amount{
			Currency:    "USD",
			AmountCents: 1000,
		},
	}

	obj1 := &model.MembershipV2Data{
		Products:        []*model.MembershipV2PurchasedProduct{product},
		NextInvoice:     invoice,
		TeamOwnerID:     "team_123",
		PaymentProvider: 1,
	}

	obj2 := &model.MembershipV2Data{
		Products:        []*model.MembershipV2PurchasedProduct{product},
		NextInvoice:     invoice,
		TeamOwnerID:     "team_123",
		PaymentProvider: 1,
	}

	result := membershipV2DataEqual(obj1, obj2)
	if !result {
		t.Logf("obj1: %+v", obj1)
		t.Logf("obj2: %+v", obj2)
	}
	require.True(t, result, "Identical objects should be equal")

	// Test nil vs empty slice edge case - should now be equal
	productWithEmptySlices := &model.MembershipV2PurchasedProduct{
		Product: &model.MembershipV2Product{
			Id:            "prod_123",
			Name:          "Test Product",
			Description:   "Test Description",
			IsTopLevel:    true,
			IsHidden:      false,
			IsIntro:       false,
			IsUpgradeable: true,
			PricesYearly:  []*model.MembershipV2Amount{}, // empty slice instead of nil
			PricesMonthly: []*model.MembershipV2Amount{}, // empty slice instead of nil
			ColorStr:      "blue",
			Offer:         "test_offer",
			Features: &model.MembershipV2Features{
				StorageBytes:  1000000,
				SpaceReaders:  5,
				SpaceWriters:  2,
				SharedSpaces:  3,
				TeamSeats:     10,
				AnyNameCount:  2,
				AnyNameMinLen: 3,
			},
		},
		PurchaseInfo: &model.MembershipV2PurchaseInfo{
			DateStarted: 1234567890,
			DateEnds:    1234567891,
			IsAutoRenew: true,
			Period:      1,
		},
		ProductStatus: &model.MembershipV2ProductStatus{
			Status: 1, // Active
		},
	}

	objWithEmptySlices := &model.MembershipV2Data{
		Products:        []*model.MembershipV2PurchasedProduct{productWithEmptySlices},
		NextInvoice:     invoice,
		TeamOwnerID:     "team_123",
		PaymentProvider: 1,
	}

	objWithNilSlices := &model.MembershipV2Data{
		Products:        []*model.MembershipV2PurchasedProduct{productWithNilSlices},
		NextInvoice:     invoice,
		TeamOwnerID:     "team_123",
		PaymentProvider: 1,
	}

	// WARNING: yes, nil and empty slices ARE NOT EQUAL!
	require.False(t, membershipV2DataEqual(objWithEmptySlices, objWithNilSlices), "Objects with different products (nil vs empty price arrays) should not be equal")

	// Test objects with different products
	obj3 := &model.MembershipV2Data{
		Products:        []*model.MembershipV2PurchasedProduct{},
		NextInvoice:     invoice,
		TeamOwnerID:     "team_123",
		PaymentProvider: 1,
	}
	require.False(t, membershipV2DataEqual(obj1, obj3), "Objects with different product counts should not be equal")

	// Test objects with different team owner ID - should now be equal since we only compare products
	obj4 := &model.MembershipV2Data{
		Products:        []*model.MembershipV2PurchasedProduct{product},
		NextInvoice:     invoice,
		TeamOwnerID:     "team_456",
		PaymentProvider: 1,
	}
	require.True(t, membershipV2DataEqual(obj1, obj4), "Objects with different team owner IDs should be equal (only products compared)")
}
