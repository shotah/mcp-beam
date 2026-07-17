package buildinfo

// Version is the mcp-beam release identity.
// Overridden at link time via:
//
//	-X go2tv.app/mcp-beam/internal/buildinfo.Version=<semver>
//
// Local/dev builds fall back to the VERSION file default when unset by ldflags.
var Version = "1.0.0"
