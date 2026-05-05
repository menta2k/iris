package providers

import "os"

// osLookupEnv is `os.LookupEnv` extracted to a package-private symbol so the
// wire-generated injector can be tested without poking the real env. Tests
// can reassign it; production code calls os.LookupEnv directly.
var osLookupEnv = os.LookupEnv
