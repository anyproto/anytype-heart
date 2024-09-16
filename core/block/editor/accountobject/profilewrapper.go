package accountobject

import "github.com/valyala/fastjson"

type profileWrapper struct {
	profile ProfileDetails
	val     *fastjson.Value
}

func newProfileWrapper(val *fastjson.Value) profileWrapper {
	return profileWrapper{
		val: val,
	}
}

func (p profileWrapper) Profile() ProfileDetails {
	return ProfileDetails{
		Name:        string(p.val.GetStringBytes("name")),
		Description: string(p.val.GetStringBytes("description")),
		IconImage:   string(p.val.GetStringBytes("iconImage")),
	}
}
