package model

func (this *MembershipTierData) Equal(that *MembershipTierData) bool {
	return deriveEqualMembershipTierData(this, that)
}

func (this *Membership) Equal(that *Membership) bool {
	return deriveEqualMembership(this, that)
}
