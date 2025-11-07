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
		ColorStr:                   "#ff00ff",
		StripeProductId:            "stripe-prod-id",
		StripeManageUrl:            "https://stripe.example/manage",
		IosProductId:               "ios-prod-id",
		IosManageUrl:               "https://ios.example/manage",
		AndroidProductId:           "android-prod-id",
		AndroidManageUrl:           "https://android.example/manage",
		Offer:                      "Launch offer",
		PriceStripeUsdCentsMonthly: 2100,
		IsIntroPlan:                true,
		IsUpgradeable:              true,
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
		ColorStr:                   src.ColorStr,
		StripeProductId:            src.StripeProductId,
		StripeManageUrl:            src.StripeManageUrl,
		IosProductId:               src.IosProductId,
		IosManageUrl:               src.IosManageUrl,
		AndroidProductId:           src.AndroidProductId,
		AndroidManageUrl:           src.AndroidManageUrl,
		Offer:                      src.Offer,
		PriceStripeUsdCentsMonthly: src.PriceStripeUsdCentsMonthly,
		IsIntroPlan:                src.IsIntroPlan,
		IsUpgradeable:              src.IsUpgradeable,
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
		IsMonthly:             true,
		TeamOwner:             "team-owner.any",
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
		IsMonthly:             src.IsMonthly,
		TeamOwner:             src.TeamOwner,
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
			IsYearly:    true,
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
			IsYearly:    src.PurchaseInfo.IsYearly,
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
