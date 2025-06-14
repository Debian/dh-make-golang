# Development Guide for dh-make-golang

This document provides comprehensive information about the development structure, contribution guidelines, and file organization of the dh-make-golang project.

## Project Overview

dh-make-golang is a tool that automates the creation of Debian packaging for Go packages. It follows the [pkg-go packaging guidelines](https://go-team.pages.debian.net/packaging.html) and is designed to simplify the process of creating Debian packages for Go libraries and programs.

## Repository Structure

The project is organized as follows:

```
dh-make-golang/
├── cmd/                  # Command implementation files
│   ├── check_depends.go  # Check dependencies command
│   ├── clone.go          # Clone command
│   ├── completion.go     # Shell completion
│   ├── make.go           # Main make command
│   ├── root.go           # Root command definition
│   └── ...
├── .github/              # GitHub-specific files
│   └── workflows/        # CI/CD workflow definitions
├── main.go               # Application entry point
├── make.go               # Package creation logic
├── version.go            # Version information
├── go.mod                # Go module definition
├── go.sum                # Go module checksums
└── ...                   # Other utility files
```

### Key Components

- **cmd/**: Contains the implementation of all CLI commands using the Cobra framework
- **main.go**: Entry point that initializes the application and GitHub client
- **make.go**: Core functionality for creating Debian packages
- **version.go**: Version information and management
- **description.go**: Package description handling
- **clone.go**: Repository cloning functionality
- **estimate.go**: Dependency estimation functionality

## Build and Run

### Build from source

```bash
# Clone the repository
git clone https://github.com/Debian/dh-make-golang.git
cd dh-make-golang

# Build the binary
go build -o dh-make-golang

# Install (optional)
sudo install -m755 dh-make-golang /usr/local/bin/
```

### Run

```bash
# Basic usage
dh-make-golang make github.com/example/package

# Get help
dh-make-golang --help

# Get help for a specific command
dh-make-golang make --help
```

## Development Workflow

### Setting up the Development Environment

1. Ensure you have Go 1.21+ installed
2. Clone the repository
3. Install development dependencies:
   ```bash
   go get -v ./...
   ```

### Using the Makefile

The project includes a Makefile to simplify common development tasks. To see all available commands:

```bash
make help
```

#### Available Make Commands

| Command | Description | Example |
|---------|-------------|---------|
| `make build` | Builds the binary | `make build` |
| `make test` | Runs all tests | `make test` |
| `make lint` | Runs formatting and linting checks | `make lint` |
| `make fmt` | Formats code using gofmt | `make fmt` |
| `make vet` | Runs go vet for static analysis | `make vet` |
| `make clean` | Removes build artifacts | `make clean` |
| `make install` | Installs binary to /usr/local/bin | `make install` |
| `make man` | Generates man page from markdown | `make man` |

### Testing

The project uses Go's standard testing framework. Run tests with:

```bash
make test
```

Or directly with Go:

```bash
go test -v ./...
```

### Code Style

The project follows Go standard code style. Before submitting changes:

1. Format your code:
   ```bash
   make fmt
   ```
2. Run linting:
   ```bash
   make lint
   ```

### Environment Variables

The following environment variables can be used during development:

- `GITHUB_USERNAME`: GitHub username for API authentication
- `GITHUB_PASSWORD`: GitHub password or token for API authentication
- `GITHUB_OTP`: One-time password for GitHub 2FA (if enabled)

## Continuous Integration

The project uses GitHub Actions for CI/CD. The workflow:

1. Runs on multiple Go versions (1.21, 1.22)
2. Checks code formatting
3. Builds the project
4. Runs tests
5. Performs static analysis
6. Tests package creation with a real-world example

## Making Contributions

### Pull Request Process

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests and ensure they pass
5. Submit a pull request

### Commit Guidelines

- Use clear, descriptive commit messages
- Reference issue numbers when applicable
- Keep commits focused on single changes

### Documentation

When adding new features or modifying existing ones:

1. Update relevant documentation
2. Add comments to complex code sections
3. Update the man page if necessary (generated from dh-make-golang.md)

## Release Process

1. Update version information in version.go
2. Update changelog
3. Create a tag for the new version
4. Build and test the release
5. Push the tag to trigger release workflows

## Additional Resources

- [pkg-go packaging guidelines](https://go-team.pages.debian.net/packaging.html)
- [Debian Salsa](https://wiki.debian.org/Salsa)
