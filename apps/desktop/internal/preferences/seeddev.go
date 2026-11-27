//go:build !prodseed

package preferences

// devSeedSalt is the development stand-in. A build without the prodseed tag
// keeps a salt fixed in the source, which is only good enough to keep a
// developer's release-shaped build loadable: anyone reading the repository can
// reopen a seed sealed with it. The release workflow builds with the tag and
// supplies its own, see seedsalt.go.
//
// It is applied here rather than only in seedSalt because seedSalt itself lives
// in two files that the tags swap; a build that compiles neither of them would
// otherwise open no seed at all. See SeedSalt.
const devSeedSalt = "luvia-dev-seed-not-for-release!!"

func seedSalt() []byte {
	return []byte(devSeedSalt)
}

// seedSaltSize matches the release build, so both seal the same layout.
const seedSaltSize = 32

// SeedSalt exposes the salt this build opens and writes seeds with, so the
// packaging step seals under the same one. The release build supplies its own
// through seedsalt.go; either way the packer and the app must agree.
func SeedSalt() []byte { return seedSalt() }
