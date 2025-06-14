% DH-MAKE-GOLANG(1) 2025-06-14

# NAME

dh-make-golang - automatically creates Debian packaging for Go packages

# SYNOPSIS

**dh-make-golang** [*globalflags*] <*command*> [*flags*] <*args*>

# DESCRIPTION

**dh-make-golang** is a tool to automatically create Debian packaging for Go
packages. Its goal is to automate away as much of the work as possible when
creating a Debian package for a Go library package or Go program.

For backwards compatibility, when no command is specified, the **make**
command is executed. To learn more about a command, run
"dh-make-golang <*command*> --help", for example "dh-make-golang make --help".

# COMMANDS

**make** [*flags*] *go-package-importpath*
:   Create a Debian package. **dh-make-golang** will create new files and
    directories in the current working directory. It will connect to
    the internet to download the specified Go package.

    Common flags:
    
    **--git-revision**=*revision*
    :   Specify a git revision to check out.
    
    **--type**=*type*
    :   Set package type (library, program, library+program, program+library).
    
    **--program-package-name**=*name*
    :   Override the program package name.
    
    **--force-prerelease**
    :   Package @master or @tip instead of the latest tagged version.
    
    **--pristine-tar**
    :   Keep using a pristine-tar branch as in the old workflow.

**search** *pattern*
:   Search Debian for already-existing packages. Uses Go's default
    regexp syntax (https://golang.org/pkg/regexp/syntax/).

**estimate** [*flags*] *go-package-importpath*
:   Estimates the work necessary to bring *go-package-importpath*
    into Debian by printing all currently unpacked repositories.
    
    Flags:
    
    **--git-revision**=*revision*
    :   Specify a git revision to estimate.

**create-salsa-project** *project-name*
:   Create a project for hosting Debian packaging on Salsa.

**clone** *package-name*
:   Clone a Go package from Salsa and download the appropriate tarball.

**check-depends**
:   Compare go.mod and d/control to check for changes in dependencies.

**completion** [*bash*|*zsh*|*fish*]
:   Generate shell completion scripts for dh-make-golang.

# EXAMPLES

Create a Debian package for a Go library:

```
dh-make-golang make --type library golang.org/x/oauth2
```

Create a Debian package for a Go program:

```
dh-make-golang make --type program github.com/cli/cli --program-package-name gh
```

Search for existing packages:

```
dh-make-golang search 'github.com/mattn'
```

Clone an existing package:

```
dh-make-golang clone golang-github-mmcdole-goxpp
```

Generate shell completion:

```
dh-make-golang completion bash > /etc/bash_completion.d/dh-make-golang
```

# ENVIRONMENT VARIABLES

**GITHUB_USERNAME**, **GITHUB_PASSWORD**, **GITHUB_OTP**
:   GitHub credentials for API authentication. Used when accessing GitHub repositories.

# SEE ALSO

**dh**(1), **dh_golang**(1), **Debian::Debhelper::Buildsystem::golang**(3pm), **wrap-and-sort**(1)

# AUTHOR

This manual page was written by Michael Stapelberg <stapelberg@debian.org>
and Dr.\ Tobias Quathamer <toddy@debian.org>,
for the Debian project (and may be used by others).
