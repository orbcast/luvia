package preferences

import "log"

// ApplyAccessKeys puts the values a rule asks for by name into the process
// environment, which the engine reads: an application opened from the Finder,
// the Dock or the Start menu inherits no shell profile. The settings page comes
// first, the release's sealed seed last; both defer to a value the environment
// already defines, so a reader who supplies a key always keeps it.
//
// This lives here rather than in the entry point because the seed's reporting
// belongs with the code that opens it: every way the seed can fail leaves the
// app running and asking for the key, which looks the same as a build that
// shipped without one.
func ApplyAccessKeys(userData, resources string) {
	ApplySecrets(LoadSecrets(userData))
	bundled, problem := InspectSeed(resources)
	if problem != SeedMissing {
		// The only place a rotated salt or a seed from another release becomes
		// visible. It never prints the key.
		log.Printf("bundled seed %s: %s", resources, problem)
	}
	if bundled != "" {
		ApplySecrets(map[string]string{SeedVariable(): bundled})
	}
}
