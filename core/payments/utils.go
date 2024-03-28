package payments

import (
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/net/idna"
)

var (
	// p       = idna.New(idna.MapForLookup(), idna.ValidateLabels(false), idna.CheckHyphens(false), idna.StrictDomainName(false), idna.Transitional(false))
	pStrict = idna.New(idna.MapForLookup(), idna.ValidateLabels(false), idna.CheckHyphens(false), idna.StrictDomainName(true), idna.Transitional(false))
)

func normalize(input string) (string, error) {
	// output, err := p.ToUnicode(input)

	// somehow "github.com/wealdtech/go-ens/v3" used non-strict version of idna
	// let's use pStrict instead of p
	output, err := pStrict.ToUnicode(input)
	if err != nil {
		return "", errors.Wrap(err, "failed to convert to standard unicode")
	}
	// If the name started with a period then ToUnicode() removes it, but we want to keep it.
	if strings.HasPrefix(input, ".") && !strings.HasPrefix(output, ".") {
		output = "." + output
	}

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
