# remote
### Execute remote commands similar to `SSH <server> command` for Enterprise Linux execution using multiplexing and simplifying the command.

## Deps:

Install Golang, and use `go build` to pull the library dependencies you need:

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

## Production Build Command:

```bash
GOOS="linux"; go build -ldflags="-s -w" # set GOOS for Windows users
```
This will build the smallest and most efficient binary possible.

## Use:

This application is meant to be soft-linked with the name of the server it will forward execution to (ensure the soft-link is on user's PATH):

```bash
ln -s remote someserver
someserver echo hello world
```
