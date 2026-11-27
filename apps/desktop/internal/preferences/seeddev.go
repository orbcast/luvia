//go:build !prodseed

package preferences

// seedSalt is the development stand-in. A build without the prodseed tag keeps a
// salt fixed in the source, which is only good enough to keep a developer's
// release-shaped build loadable: anyone reading the repository can reopen a seed
// sealed with it. The release workflow builds with the tag and supplies its own,
// see seedsalt.go.
func seedSalt() []byte {
	return []byte("luvia-dev-seed-not-for-release!!")
}

// seedSaltSize matches the release build, so both seal the same layout.
const seedSaltSize = 32
