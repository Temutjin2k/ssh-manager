# ssh-manager

Pick an SSH connection from a menu with the arrow keys and get straight into the server.

## Install

Requires Go and `sshpass` (only for connections with a password).

```bash
go install github.com/Temutjin2k/ssh-manager@latest
```

The binary lands in `$(go env GOPATH)/bin` (usually `~/go/bin`); make sure it is in your `PATH`.

## Config

```bash
mkdir -p ~/.config/ssh-manager
curl -o ~/.config/ssh-manager/conf.yaml https://raw.githubusercontent.com/Temutjin2k/ssh-manager/main/conf.example.yaml
chmod 600 ~/.config/ssh-manager/conf.yaml
```

Edit the file and list your connections (`port` is optional). Use `-c path` for another config file.

## Usage

```bash
ssh-manager
```

↑/↓ (or j/k) to move, Enter to connect, q to quit.
