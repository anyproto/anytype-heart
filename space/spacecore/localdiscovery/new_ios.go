package localdiscovery

// New picks the iOS discovery for this account session (see
// newForInstalledProvider). The app installs its proxy once at launch, before
// any account starts, and New runs in Bootstrap at account start, so the proxy
// is already known here; one installed later applies from the next account
// start.
func New() LocalDiscovery {
	return newForInstalledProvider()
}
