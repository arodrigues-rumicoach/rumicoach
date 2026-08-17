// Package templates embeds the transactional email templates into the binary,
// so rendering never depends on the process working directory (go run from any
// directory, tests, and the Docker image all behave the same).
package templates

import "embed"

//go:embed emails/*.html
var Emails embed.FS
