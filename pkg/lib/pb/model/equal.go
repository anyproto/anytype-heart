package model

func (this *MembershipTierData) Equal(that *MembershipTierData) bool {
	return deriveEqualMembershipTierData(this, that)
}

func (this *Membership) Equal(that *Membership) bool {
	return deriveEqualMembership(this, that)
}

func (this *MembershipV2Product) Equal(that *MembershipV2Product) bool {
	if this == nil && that == nil {
		return true
	}
	if this == nil || that == nil {
		return false
	}
	return this.Id == that.Id &&
		this.Name == that.Name &&
		this.Description == that.Description &&
		this.IsTopLevel == that.IsTopLevel &&
		this.IsHidden == that.IsHidden &&
		this.IsIntro == that.IsIntro &&
		this.IsUpgradeable == that.IsUpgradeable &&
		this.ColorStr == that.ColorStr &&
		this.Offer == that.Offer
}
