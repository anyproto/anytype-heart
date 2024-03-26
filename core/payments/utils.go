package payments

import (
	ens "github.com/wealdtech/go-ens/v3"
)

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
	name, err := ens.Normalize(name)
	if err != nil {
		return name, err
	}

	return name, nil
}
