# remote

## sys_remote.pl alternative for Enterprise Linux execution.

## Deps:

Install Golang, and use `go build` to pull the dependencies you need:

### RHEL 8:
```bash
sudo yum module install go-toolset -y
```

### RHEL 9:
```bash
sudo dnf install go-toolset -y
```

### Debian/Ubuntu:
```bash
sudo yum module install go-toolset -y
```

### Arch:
```bash
sudo yum module install go-toolset -y
```

### Windows:
https://go.dev/dl/

## Production build command:

```bash
GOOS="linux"; go build -ldflags="-s -w" # set GOOS for Windows users
```
This will build the smallest and most efficient binary possible.

## Use:

This application is meant to be soft-linked with the name of the server it will forward execution to:

```bash
ln -s remote someserver
someserver echo hello world
```
