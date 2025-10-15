package payments

import (
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/net/idna"

	"github.com/anyproto/any-sync/paymentservice/paymentserviceproto"

	"github.com/anyproto/anytype-heart/core/nameservice"
	"github.com/anyproto/anytype-heart/pb"
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

func convertMembershipStatus(status *paymentserviceproto.GetSubscriptionResponse) pb.RpcMembershipGetStatusResponse {
	return pb.RpcMembershipGetStatusResponse{
		Data: convertMembershipData(status),
		Error: &pb.RpcMembershipGetStatusResponseError{
			Code: pb.RpcMembershipGetStatusResponseError_NULL,
		},
	}
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

func convertTierData(src *paymentserviceproto.TierData) *model.MembershipTierData {
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
	out.Features = make([]string, len(src.Features))
	for i, feature := range src.Features {
		out.Features[i] = feature.Description
	}

	return out
}

func convertMembershipData(src *paymentserviceproto.GetSubscriptionResponse) *model.Membership {
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
	return out
}
