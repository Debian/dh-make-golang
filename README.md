[![Build Status](https://travis-ci.org/Debian/dh-make-golang.svg?branch=master)](https://travis-ci.org/Debian/dh-make-golang)

dh-make-golang is a tool to automatically create Debian packaging for Go
packages. Its goal is to automate away as much of the work as possible when
creating a Debian package for a Go library package or Go program.

## Overview

All you need to specify is a Go package name. In your current working
directory, a new directory will be created containing a git repository. Inside
that repository, you’ll find the Go package’s source code plus the necessary
Debian packaging files to build a Debian package. The packaging adheres to the
[pkg-go packaging guidelines](https://go-team.pages.debian.net/packaging.html)
and hence can be placed alongside the other [team-maintained packages in
pkg-go](https://salsa.debian.org/go-team/packages), hosted on Debian’s
[salsa](https://wiki.debian.org/Salsa).

## Usage/example

For an introductory example, see [this annotated demonstration of how to use
dh-make-golang](https://people.debian.org/~stapelberg/2015/07/27/dh-make-golang.html).

## dh-make-golang’s usage of the internet

dh-make-golang makes heavy use of online resources to improve the resulting
package. In no particular order and depending on where your package is hosted,
dh-make-golang may query:

* By virtue of using `go get`, the specified Go package and all of its
  dependencies will be downloaded. This step can quickly cause dozens of
  megabytes to be transferred, so be careful if you are on a metered internet
  connection.
* The output of
  [filter-packages.sh](https://github.com/Debian/dh-make-golang/blob/master/filter-packages.sh),
  hosted on https://people.debian.org/~stapelberg/dh-make-golang/. This is used
  to figure out whether dependencies are already packaged in Debian, and
  whether you are about to duplicate somebody else’s work.
* GitHub’s API, to get the license, repository creator, description and README
  for Go packages hosted on GitHub.
