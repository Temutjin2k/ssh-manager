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

Edit the file and list your connections (`port` and `sudo` are optional). Use `-c path` for another config file.

## Usage

```bash
ssh-manager
```

↑/↓ (or j/k) to move, Enter to connect, q to quit.

The first entry, `+ add new connection?`, asks for name, user, host, port, password and whether to run sudo, then appends the connection to the config file. If no config exists yet, `~/.config/ssh-manager/conf.yaml` is created.

On the selected connection, `[edit]` (yellow) and `[delete]` (red) appear to the right. Press → / ← to focus them and Enter to run. Editing pre-fills the current values (an empty password keeps the current one); deleting asks for confirmation.

## Sudo

For connections with a `password`, right after login the tool runs `sudo -S -v` with that password, then drops you into a normal shell. Sudo's credentials are cached for the session, so later `sudo` commands don't ask for the password again (until the server's sudo timeout expires).

This assumes the sudo password is the same as the SSH password. To turn it off for a connection, set `sudo: false`:

```yaml
connections:
  my_server:
    user: user
    host: 192.168.2.80
    password: qwerty
    sudo: false
```

## Disclaimer

This project was fully generated with AI.

Passwords in the config are stored in plain text — keep the file private (`chmod 600`) and prefer SSH keys where possible.
