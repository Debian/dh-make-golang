package main

import (
	"fmt"
	"runtime"
)

// Version represents the dh-make-golang build version.
type Version struct {
	major      int
	minor      int
	patch      int
	preRelease string
}

var currentVersion = Version{
	major:      0,
	minor:      4,
	patch:      0,
	preRelease: "",
}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d%s", v.major, v.minor, v.patch, v.preRelease)
}

func buildVersionString() string {
	version := "v" + currentVersion.String()
	osArch := runtime.GOOS + "/" + runtime.GOARCH
	return fmt.Sprintf("%s %s %s", program, version, osArch)
}
