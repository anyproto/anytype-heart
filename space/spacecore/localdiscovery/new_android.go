package localdiscovery

// New returns the provider-backed discovery: Android discovers through
// NsdManager in the app, behind the clientlibrary DiscoveryProxy bridge.
func New() LocalDiscovery {
	return newProvider()
}
