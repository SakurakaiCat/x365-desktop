//go:build !windows && !darwin

package main

import _ "embed"

//go:embed frontend/src/assets/icon.png
var appIcon []byte
