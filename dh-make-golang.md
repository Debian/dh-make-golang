% DH-MAKE-GOLANG(1) 2018-09-15

# NAME

dh-make-golang - automatically creates Debian packaging for Go packages

# SYNOPSIS

**dh-make-golang** [*globalflags*] <*command*> [*flags*] <*args*>

# DESCRIPTION

**dh-make-golang** is a tool to automatically create Debian packaging for Go
packages. Its goal is to automate away as much of the work as possible when
creating a Debian package for a Go library package.

For backwards compatibility, when no command is specified, the **make**
command is executed. To learn more about a command, run
"dh-make-golang <*command*> -help", for example "dh-make-golang make -help".

# COMMANDS

**make** *go-package-importpath*
:   Create a Debian package. **dh-make-golang** will create new files and
    directories in the current working directory. It will connect to
    the internet to download the specified Go package.

**search** *pattern*
:   Search Debian for already-existing packages. Uses Go's default
    regexp syntax (https://golang.org/pkg/regexp/syntax/).

**estimate** *go-package-importpath*
:   Estimates the work necessary to bring *go-package-importpath*
    into Debian by printing all currently unpacked repositories.

**create-salsa-project** *project-name*
:   Create a project for hosting Debian packaging.

# OPTIONS

Run **dh-make-golang** -help for more details.

# AUTHOR

This manual page was written by Michael Stapelberg <stapelberg@debian.org>
and Dr.\ Tobias Quathamer <toddy@debian.org>,
for the Debian project (and may be used by others).
