# Architecture

## Services/components
### Bootstrapping
If you need your component to be visible to other components, you need to register it in Service Locator's registry. 
To do it go to `Bootstrap` function in `core/anytype/bootstrap.go` and `Register` it

### Dependency injection
We use our own implementation of Service Locator pattern: `github.com/anyproto/any-sync/app`.