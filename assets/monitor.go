// Package assets shares the README artwork with the local monitor.
package assets

import _ "embed"

//go:embed readme/logo.jpg
var MonitorLogo []byte

//go:embed readme/hero.jpg
var MonitorHero []byte
