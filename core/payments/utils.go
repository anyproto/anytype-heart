package payments

import (
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/net/idna"

	"github.com/anyproto/any-sync/paymentservice/paymentserviceproto"

	"github.com/anyproto/anytype-heart/core/nameservice"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var (
	// p       = idna.New(idna.MapForLookup(), idna.ValidateLabels(false), idna.CheckHyphens(false), idna.StrictDomainName(false), idna.Transitional(false))
	pStrict = idna.New(idna.MapForLookup(), idna.ValidateLabels(false), idna.CheckHyphens(false), idna.StrictDomainName(true), idna.Transitional(false))
)

func normalize(input string) (string, error) {
	// output, err := p.ToUnicode(input)
	// if name has no .any suffix -> error
	if len(input) < 4 || input[len(input)-4:] != ".any" {
		return "", errors.New("name must have .any suffix")
	}
	// remove .any suffix
	input = input[:len(input)-4]

	// somehow "github.com/wealdtech/go-ens/v3" used non-strict version of idna
	// let's use pStrict instead of p
	output, err := pStrict.ToUnicode(input)
	if err != nil {
		return "", errors.Wrap(err, "failed to convert to standard unicode")
	}
	if strings.Contains(input, ".") {
		return "", errors.New("name cannot contain a period")
	}

	// add .any suffix
	output += ".any"

	return output, nil
}

func normalizeAnyName(name string) (string, error) {
	// 1. ENSIP1 standard: ens-go v3.6.0 (current) is using it
	// 2. ENSIP15 standard: that is an another standard for ENS namehashes
	// that was accepted in June 2023.
	//
	// Current AnyNS (as of February 2024) implementation support only ENSIP1
	//
	// https://eips.ethereum.org/EIPS/eip-137 (ENSIP1) grammar:
	// <domain> ::= <label> | <domain> "." <label>
	// <label> ::= any valid string label per [UTS46](https://unicode.org/reports/tr46/)
	//
	// "❶❷❸❹❺❻❼❽❾❿":
	// 	under ENSIP1 this OK
	// 	under ENSIP15 this is not OK, will fail

	// from "github.com/wealdtech/go-ens/v3"
	// name, err := ens.Normalize(name)

	name, err := normalize(name)

	if err != nil {
		return name, err
	}

	return name, nil
}

func tiersAreEqual(a []*model.MembershipTierData, b []*model.MembershipTierData) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Equal(b[i]) {
			return false
		}
	}
	return true
}

func productsV2Equal(a []*model.MembershipV2Product, b []*model.MembershipV2Product) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Equal(b[i]) {
			return false
		}
	}
	return true
}

func convertTierData(src *paymentserviceproto.TierData) *model.MembershipTierData {
	if src == nil {
		return nil
	}
	out := &model.MembershipTierData{}
	out.Id = src.Id
	out.Name = src.Name
	out.Description = src.Description
	out.IsTest = src.IsTest
	out.PeriodType = model.MembershipTierDataPeriodType(src.PeriodType)
	out.PeriodValue = src.PeriodValue
	out.PriceStripeUsdCents = src.PriceStripeUsdCents
	out.AnyNamesCountIncluded = src.AnyNamesCountIncluded
	out.AnyNameMinLength = src.AnyNameMinLength
	out.ColorStr = src.ColorStr
	out.StripeProductId = src.StripeProductId
	out.StripeManageUrl = src.StripeManageUrl
	out.IosProductId = src.IosProductId
	out.IosManageUrl = src.IosManageUrl
	out.AndroidProductId = src.AndroidProductId
	out.AndroidManageUrl = src.AndroidManageUrl
	out.Offer = src.Offer
	out.PriceStripeUsdCentsMonthly = src.PriceStripeUsdCentsMonthly
	out.IsIntroPlan = src.IsIntroPlan
	out.IsUpgradeable = src.IsUpgradeable

	if src.Features != nil {
		out.Features = make([]string, len(src.Features))
		for i, feature := range src.Features {
			out.Features[i] = feature.Description
		}
	}
	return out
}

func convertMembershipData(src *paymentserviceproto.GetSubscriptionResponse) *model.Membership {
	if src == nil {
		return nil
	}
	out := &model.Membership{}
	out.Tier = src.Tier
	out.Status = model.MembershipStatus(src.Status)
	out.DateStarted = src.DateStarted
	out.DateEnds = src.DateEnds
	out.IsAutoRenew = src.IsAutoRenew
	out.PaymentMethod = PaymentMethodToModel(src.PaymentMethod)
	out.UserEmail = src.UserEmail
	out.SubscribeToNewsletter = src.SubscribeToNewsletter
	out.NsName, out.NsNameType = nameservice.FullNameToNsName(src.RequestedAnyName)
	out.IsMonthly = src.IsMonthly
	out.TeamOwner = src.TeamOwner
	return out
}

func convertAmountData(src *paymentserviceproto.MembershipV2_Amount) *model.MembershipV2Amount {
	if src == nil {
		return nil
	}
	out := &model.MembershipV2Amount{}
	out.Currency = src.Currency
	out.AmountCents = src.AmountCents
	return out
}

func convertProductData(src *paymentserviceproto.MembershipV2_Product) *model.MembershipV2Product {
	if src == nil {
		return nil
	}
	out := &model.MembershipV2Product{}
	out.Id = src.Id
	out.Name = src.Name
	out.Description = src.Description
	out.IsTopLevel = src.IsTopLevel
	out.IsHidden = src.IsHidden
	out.IsIntro = src.IsIntro
	out.IsUpgradeable = src.IsUpgradeable
	out.ColorStr = src.ColorStr
	out.Offer = src.Offer

	out.PricesYearly = make([]*model.MembershipV2Amount, len(src.PricesYearly))
	for i, price := range src.PricesYearly {
		out.PricesYearly[i] = convertAmountData(price)
	}
	out.PricesMonthly = make([]*model.MembershipV2Amount, len(src.PricesMonthly))
	for i, price := range src.PricesMonthly {
		out.PricesMonthly[i] = convertAmountData(price)
	}

	if src.Features != nil {
		out.Features = &model.MembershipV2Features{}

		out.Features.StorageBytes = src.Features.StorageBytes
		out.Features.SpaceReaders = src.Features.SpaceReaders
		out.Features.SpaceWriters = src.Features.SpaceWriters
		out.Features.SharedSpaces = src.Features.SharedSpaces
		out.Features.TeamSeats = src.Features.TeamSeats
		out.Features.AnyNameCount = src.Features.AnyNameCount
		out.Features.AnyNameMinLen = src.Features.AnyNameMinLen
	}
	return out
}

func convertPurchaseInfoData(src *paymentserviceproto.MembershipV2_PurchaseInfo) *model.MembershipV2PurchaseInfo {
	if src == nil {
		return nil
	}
	out := &model.MembershipV2PurchaseInfo{}
	out.DateStarted = src.DateStarted
	out.DateEnds = src.DateEnds
	out.IsAutoRenew = src.IsAutoRenew
	out.IsYearly = src.IsYearly
	return out
}

func convertProductStatusData(src *paymentserviceproto.MembershipV2_ProductStatus) *model.MembershipV2ProductStatus {
	if src == nil {
		return nil
	}
	out := &model.MembershipV2ProductStatus{}
	// 1-1 conversion
	out.Status = model.MembershipV2ProductStatusStatus(src.Status)
	return out
}

func convertPurchasedProductData(src *paymentserviceproto.MembershipV2_PurchasedProduct) *model.MembershipV2PurchasedProduct {
	if src == nil {
		return nil
	}
	out := &model.MembershipV2PurchasedProduct{}
	out.Product = convertProductData(src.Product)
	out.PurchaseInfo = convertPurchaseInfoData(src.PurchaseInfo)
	out.ProductStatus = convertProductStatusData(src.ProductStatus)
	return out
}

func convertInvoiceData(src *paymentserviceproto.MembershipV2_Invoice) *model.MembershipV2Invoice {
	if src == nil {
		return nil
	}
	out := &model.MembershipV2Invoice{}
	out.Date = src.Date
	if src.Total != nil {
		out.Total = &model.MembershipV2Amount{}
		out.Total.Currency = src.Total.Currency
		out.Total.AmountCents = src.Total.AmountCents
	}
	return out
}

func convertCartData(src *paymentserviceproto.MembershipV2_StoreCartGetResponse) *model.MembershipV2Cart {
	if src == nil {
		return nil
	}
	out := &model.MembershipV2Cart{}
	out.Products = make([]*model.MembershipV2CartProduct, len(src.Cart.Products))
	for i, product := range src.Cart.Products {
		out.Products[i] = convertCartProductData(product)
	}
	out.Total = convertAmountData(src.Cart.Total)
	out.TotalNextInvoice = convertAmountData(src.Cart.TotalNextInvoice)
	out.NextInvoiceDate = src.Cart.NextInvoiceDate
	return out
}

func convertCartProductData(src *paymentserviceproto.MembershipV2_CartProduct) *model.MembershipV2CartProduct {
	if src == nil {
		return nil
	}
	out := &model.MembershipV2CartProduct{}
	out.Product = convertProductData(src.Product)
	out.IsYearly = src.IsYearly
	out.Remove = src.Remove
	return out
}

func MembershipV2DataEqual(a, b *model.MembershipV2Data) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	// Compare Products length
	if len(a.Products) != len(b.Products) {
		return false
	}
	// Compare Products (simplified - just check IDs)
	for i := range a.Products {
		if a.Products[i] == nil || b.Products[i] == nil {
			if a.Products[i] != b.Products[i] {
				return false
			}
			continue
		}
		if a.Products[i].Product == nil || b.Products[i].Product == nil {
			if a.Products[i].Product != b.Products[i].Product {
				return false
			}
			continue
		}
		if a.Products[i].Product.Id != b.Products[i].Product.Id {
			return false
		}
	}
	// Compare NextInvoice
	if a.NextInvoice == nil && b.NextInvoice == nil {
		return true
	}
	if a.NextInvoice == nil || b.NextInvoice == nil {
		return false
	}
	if a.NextInvoice.Date != b.NextInvoice.Date {
		return false
	}
	return true
}
