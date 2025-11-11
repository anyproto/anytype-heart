package model

func (this *MembershipTierData) Equal(that *MembershipTierData) bool {
	return deriveEqualMembershipTierData(this, that)
}

func (this *Membership) Equal(that *Membership) bool {
	return deriveEqualMembership(this, that)
}

func (this *MembershipV2Product) Equal(that *MembershipV2Product) bool {
	return deriveEqualMembershipV2Product(this, that)
}

func (this *MembershipV2Data) Equal(that *MembershipV2Data) bool {
	return deriveEqualMembershipV2(this, that)
}
