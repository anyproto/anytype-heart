package model

func (this *MembershipTierData) Equal(that *MembershipTierData) bool {
	// only Features slice
	return deriveEqualMembershipTierData(this, that)
}

func (this *Membership) Equal(that *Membership) bool {
	// no slices here
	return deriveEqualMembership(this, that)
}

func (this *MembershipV2Product) Equal(that *MembershipV2Product) bool {
	return deriveEqualMembershipV2Product(this, that)
}

func (this *MembershipV2Data) Equal(that *MembershipV2Data) bool {
	return deriveEqualMembershipV2(this, that)
}

func (this *MembershipV2PurchasedProduct) Equal(that *MembershipV2PurchasedProduct) bool {
	return deriveEqualPurchasedProduct(this, that)
}

func (this *IdentityProfile) Equal(that *IdentityProfile) bool {
	return deriveEqualIdentityProfile(this, that)
}
