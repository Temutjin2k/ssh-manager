package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

type conn struct {
	Name     string
	User     string `yaml:"user"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Password string `yaml:"password"`
}

func main() {
	path := flag.String("c", "", "path to config (default: ~/.config/ssh-manager/conf.yaml, then ./conf.yaml)")
	flag.Parse()

	conns, err := load(configPath(*path))
	if err != nil {
		fatal(err)
	}
	if len(conns) == 0 {
		fatal(fmt.Errorf("no connections in config"))
	}

	c, ok := choose(conns)
	if !ok {
		return
	}
	connect(c)
}

func configPath(flagPath string) string {
	if flagPath != "" {
		return flagPath
	}
	home, _ := os.UserHomeDir()
	global := filepath.Join(home, ".config", "ssh-manager", "conf.yaml")
	if _, err := os.Stat(global); err == nil {
		return global
	}
	return "conf.yaml"
}

func load(path string) ([]conn, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Connections yaml.Node `yaml:"connections"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}

	// Walk the node pairs instead of decoding into a map to keep file order.
	var conns []conn
	nodes := doc.Connections.Content
	for i := 0; i+1 < len(nodes); i += 2 {
		c := conn{Name: nodes[i].Value}
		if err := nodes[i+1].Decode(&c); err != nil {
			return nil, fmt.Errorf("%s: %w", c.Name, err)
		}
		conns = append(conns, c)
	}
	return conns, nil
}

var palette = []int{203, 215, 227, 114, 80, 75, 141, 213}

func color(i int) string {
	return fmt.Sprintf("\033[38;5;%dm", palette[i%len(palette)])
}

func choose(conns []conn) (conn, bool) {
	fd := int(os.Stdin.Fd())
	old, err := term.MakeRaw(fd)
	if err != nil {
		fatal(err)
	}
	defer func() {
		fmt.Print("\033[?25h")
		term.Restore(fd, old)
	}()

	sel := 0
	draw := func(first bool) {
		if !first {
			fmt.Printf("\033[%dA", len(conns))
		}
		for i, c := range conns {
			line := fmt.Sprintf("%s  (%s@%s)", c.Name, c.User, c.Host)
			if i == sel {
				fmt.Printf("\r\033[K%s\033[1m▶ %s\033[0m\r\n", color(i), line)
			} else {
				fmt.Printf("\r\033[K%s  %s\033[0m\r\n", color(i), line)
			}
		}
	}

	fmt.Print("\033[1mSelect connection\033[0m  \033[2m↑/↓ move · Enter connect · q quit\033[0m\r\n")
	fmt.Print("\033[?25l")
	draw(true)

	buf := make([]byte, 3)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			return conn{}, false
		}
		switch {
		case n == 3 && buf[0] == 27 && buf[1] == '[' && buf[2] == 'A', n == 1 && buf[0] == 'k':
			sel = (sel - 1 + len(conns)) % len(conns)
		case n == 3 && buf[0] == 27 && buf[1] == '[' && buf[2] == 'B', n == 1 && buf[0] == 'j':
			sel = (sel + 1) % len(conns)
		case n == 1 && (buf[0] == '\r' || buf[0] == '\n'):
			return conns[sel], true
		case n == 1 && (buf[0] == 'q' || buf[0] == 3):
			return conn{}, false
		}
		draw(false)
	}
}

func connect(c conn) {
	target := c.Host
	if c.User != "" {
		target = c.User + "@" + c.Host
	}
	// sshpass exits silently (code 6) on the unknown-host-key prompt, so never let ssh ask.
	args := []string{"ssh", "-o", "StrictHostKeyChecking=accept-new"}
	if c.Port != 0 {
		args = append(args, "-p", fmt.Sprint(c.Port))
	}
	args = append(args, target)

	if c.Password != "" {
		if _, err := exec.LookPath("sshpass"); err != nil {
			fmt.Fprintln(os.Stderr, "sshpass not found, falling back to manual password prompt")
		} else {
			args = append([]string{"sshpass", "-p", c.Password}, args...)
		}
	}

	bin, err := exec.LookPath(args[0])
	if err != nil {
		fatal(err)
	}
	// Replace this process so the shell returns to the user's terminal on exit.
	fatal(syscall.Exec(bin, args, os.Environ()))
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
