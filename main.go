package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unicode/utf8"

	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

type conn struct {
	Name     string
	User     string `yaml:"user"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Password string `yaml:"password"`
	Sudo     *bool  `yaml:"sudo"`
}

func main() {
	flagPath := flag.String("c", "", "path to config (default: ~/.config/ssh-manager/conf.yaml, then ./conf.yaml)")
	flag.Parse()
	path := configPath(*flagPath)

	for {
		conns, err := load(path)
		if err != nil {
			fatal(err)
		}

		act, idx, ok := choose(conns)
		if !ok {
			return
		}
		switch act {
		case actConnect:
			connect(conns[idx])
		case actAdd:
			err = saveConnection(path, conns, nil)
		case actEdit:
			err = saveConnection(path, conns, &conns[idx])
		case actDelete:
			err = deleteConnection(path, conns[idx])
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
		}
	}
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
	if _, err := os.Stat("conf.yaml"); err == nil {
		return "conf.yaml"
	}
	return global
}

func load(path string) ([]conn, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
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

type action int

const (
	actConnect action = iota
	actAdd
	actEdit
	actDelete
)

func choose(conns []conn) (action, int, bool) {
	fd := int(os.Stdin.Fd())
	old, err := term.MakeRaw(fd)
	if err != nil {
		fatal(err)
	}
	defer func() {
		fmt.Print("\033[?25h")
		term.Restore(fd, old)
	}()

	// Row 0 is the "add new connection" entry, so conns[i] sits at row i+1.
	rows := len(conns) + 1
	lines := make([]string, rows)
	lines[0] = "+ add new connection?"
	width := utf8.RuneCountInString(lines[0])
	for i, c := range conns {
		lines[i+1] = fmt.Sprintf("%s  (%s@%s)", c.Name, c.User, c.Host)
		width = max(width, utf8.RuneCountInString(lines[i+1]))
	}

	// focus 0 is the row itself; 1 and 2 are the [edit] and [delete] buttons on the selected row.
	sel, focus := 0, 0
	button := func(focused bool, code, label string) string {
		if focused {
			code = "7;" + code
		}
		return "\033[" + code + "m" + label + "\033[0m"
	}
	draw := func(first bool) {
		if !first {
			fmt.Printf("\033[%dA", rows)
		}
		for row, line := range lines {
			style := "\033[38;5;250m"
			if row > 0 {
				style = color(row - 1)
			}
			marker := "  "
			if row == sel {
				marker = "\033[1m▶ "
			}
			fmt.Printf("\r\033[K%s%s%s\033[0m", style, marker, line)
			if row == sel && row > 0 {
				pad := strings.Repeat(" ", width-utf8.RuneCountInString(line)+3)
				fmt.Print(pad, button(focus == 1, "33", "[edit]"), "  ", button(focus == 2, "31", "[delete]"))
			}
			fmt.Print("\r\n")
		}
	}

	fmt.Print("\033[1mSelect connection\033[0m  \033[2m↑/↓ move · ←/→ edit/delete · Enter select · q quit\033[0m\r\n")
	fmt.Print("\033[?25l")
	draw(true)

	buf := make([]byte, 3)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			return 0, 0, false
		}
		switch {
		case n == 3 && buf[0] == 27 && buf[1] == '[' && buf[2] == 'A', n == 1 && buf[0] == 'k':
			sel, focus = (sel-1+rows)%rows, 0
		case n == 3 && buf[0] == 27 && buf[1] == '[' && buf[2] == 'B', n == 1 && buf[0] == 'j':
			sel, focus = (sel+1)%rows, 0
		case n == 3 && buf[0] == 27 && buf[1] == '[' && buf[2] == 'C', n == 1 && buf[0] == 'l':
			if sel > 0 {
				focus = min(focus+1, 2)
			}
		case n == 3 && buf[0] == 27 && buf[1] == '[' && buf[2] == 'D', n == 1 && buf[0] == 'h':
			focus = max(focus-1, 0)
		case n == 1 && (buf[0] == '\r' || buf[0] == '\n'):
			switch {
			case sel == 0:
				return actAdd, 0, true
			case focus == 1:
				return actEdit, sel - 1, true
			case focus == 2:
				return actDelete, sel - 1, true
			}
			return actConnect, sel - 1, true
		case n == 1 && (buf[0] == 'q' || buf[0] == 3):
			return 0, 0, false
		}
		draw(false)
	}
}

type entry struct {
	User     string `yaml:"user"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port,omitempty"`
	Password string `yaml:"password,omitempty"`
	Sudo     *bool  `yaml:"sudo,omitempty"`
}

func newReader() func(label, def string) string {
	in := bufio.NewReader(os.Stdin)
	return func(label, def string) string {
		if def != "" {
			label += " [" + def + "]"
		}
		fmt.Print(label + ": ")
		line, _ := in.ReadString('\n')
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
		return def
	}
}

func validPort(v string) (int, bool) {
	p, err := strconv.Atoi(v)
	return p, err == nil && p > 0 && p < 65536
}

func promptConnection(existing []conn) (string, entry, error) {
	ask := newReader()
	required := func(label string) string {
		for {
			if v := ask(label, ""); v != "" {
				return v
			}
		}
	}

	fmt.Print("\033[2J\033[H\033[1mNew connection\033[0m  \033[2m(Ctrl+C to quit)\033[0m\n\n")

	var name string
	for {
		name = required("Name")
		if !hasName(existing, name) {
			break
		}
		fmt.Println("That name already exists.")
	}
	e := entry{User: ask("User", ""), Host: required("Host")}

	for {
		if p, ok := validPort(ask("Port", "22")); ok {
			if p != 22 {
				e.Port = p
			}
			break
		}
		fmt.Println("Port must be a number from 1 to 65535.")
	}

	fmt.Print("Password (hidden, empty for key/manual): ")
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", entry{}, err
	}
	e.Password = string(pw)

	if e.Password != "" && strings.HasPrefix(strings.ToLower(ask("Run sudo after login? (Y/n)", "y")), "n") {
		off := false
		e.Sudo = &off
	}
	return name, e, nil
}

// editConnection shows every field of cur and lets the user pick which one to change.
func editConnection(existing []conn, cur conn) (string, entry, bool, error) {
	fd := int(os.Stdin.Fd())
	old, err := term.MakeRaw(fd)
	if err != nil {
		return "", entry{}, false, err
	}
	defer func() {
		fmt.Print("\033[?25h")
		term.Restore(fd, old)
	}()
	// Typing a new value needs the normal line-editing terminal, so drop raw mode around it.
	cooked := func(f func()) {
		fmt.Print("\033[?25h")
		term.Restore(fd, old)
		f()
		term.MakeRaw(fd)
		fmt.Print("\033[?25l")
	}

	name, user, host, pass := cur.Name, cur.User, cur.Host, cur.Password
	port := cur.Port
	if port == 0 {
		port = 22
	}
	sudoOn := cur.Sudo == nil || *cur.Sudo

	const (
		fName = iota
		fUser
		fHost
		fPort
		fPass
		fSudo
		fSave
		fBack
	)
	labels := []string{"Name", "User", "Host", "Port", "Password", "Sudo", "Save", "Back"}
	value := func(i int) string {
		switch i {
		case fName:
			return name
		case fUser:
			return user
		case fHost:
			return host
		case fPort:
			return strconv.Itoa(port)
		case fPass:
			if pass == "" {
				return "\033[2m(none)\033[0m"
			}
			return "••••••••"
		case fSudo:
			if sudoOn {
				return "yes"
			}
			return "no"
		}
		return ""
	}

	sel, msg := 0, ""
	draw := func() {
		fmt.Print("\033[2J\033[H")
		fmt.Print("\033[1mEdit connection\033[0m  \033[2m↑/↓ move · Enter change · s save · Esc/q back\033[0m\r\n\r\n")
		for i, l := range labels {
			marker := "  "
			if i == sel {
				marker = "\033[1m▶ "
			}
			switch i {
			case fSave:
				fmt.Print("\r\n", marker, "\033[32m", l, "\033[0m\r\n")
			case fBack:
				fmt.Print(marker, "\033[38;5;250m", l, "\033[0m\r\n")
			default:
				fmt.Printf("%s\033[33m%-10s\033[0m %s\r\n", marker, l, value(i))
			}
		}
		if msg != "" {
			fmt.Print("\r\n\033[31m", msg, "\033[0m\r\n")
		}
	}
	fmt.Print("\033[?25l")

	buf := make([]byte, 3)
	for {
		draw()
		n, err := os.Stdin.Read(buf)
		if err != nil {
			return "", entry{}, false, nil
		}
		msg = ""
		save := false
		switch {
		case n == 3 && buf[0] == 27 && buf[1] == '[' && buf[2] == 'A', n == 1 && buf[0] == 'k':
			sel = (sel - 1 + len(labels)) % len(labels)
		case n == 3 && buf[0] == 27 && buf[1] == '[' && buf[2] == 'B', n == 1 && buf[0] == 'j':
			sel = (sel + 1) % len(labels)
		case n == 1 && (buf[0] == 27 || buf[0] == 'q' || buf[0] == 3):
			return "", entry{}, false, nil
		case n == 1 && buf[0] == 's':
			save = true
		case n == 1 && (buf[0] == '\r' || buf[0] == '\n'):
			switch sel {
			case fSave:
				save = true
			case fBack:
				return "", entry{}, false, nil
			case fSudo:
				sudoOn = !sudoOn
			case fPass:
				cooked(func() {
					fmt.Print("\033[2J\033[H\033[1mPassword\033[0m (hidden, empty keeps current, - clears)\n\n> ")
					pw, _ := term.ReadPassword(fd)
					fmt.Println()
					switch string(pw) {
					case "":
					case "-":
						pass = ""
					default:
						pass = string(pw)
					}
				})
			default:
				cooked(func() {
					fmt.Printf("\033[2J\033[H\033[1m%s\033[0m (empty keeps current)\n\n", labels[sel])
					v := newReader()("> ", value(sel))
					switch sel {
					case fName:
						if v != cur.Name && hasName(existing, v) {
							msg = "That name already exists."
						} else {
							name = v
						}
					case fUser:
						user = v
					case fHost:
						host = v
					case fPort:
						if p, ok := validPort(v); ok {
							port = p
						} else {
							msg = "Port must be a number from 1 to 65535."
						}
					}
				})
			}
		}
		if save {
			e := entry{User: user, Host: host, Password: pass}
			if port != 22 {
				e.Port = port
			}
			if !sudoOn {
				off := false
				e.Sudo = &off
			}
			return name, e, true, nil
		}
	}
}

func saveConnection(path string, existing []conn, cur *conn) error {
	var (
		name  string
		e     entry
		saved = true
		err   error
	)
	if cur == nil {
		name, e, err = promptConnection(existing)
	} else {
		name, e, saved, err = editConnection(existing, *cur)
	}
	if err != nil || !saved {
		return err
	}
	val := &yaml.Node{}
	if err := val.Encode(e); err != nil {
		return err
	}
	err = updateConns(path, func(m *yaml.Node) error {
		if cur == nil {
			m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: name}, val)
			return nil
		}
		i := pairIndex(m, cur.Name)
		if i < 0 {
			return fmt.Errorf("%s not found in config", cur.Name)
		}
		val.HeadComment, val.LineComment, val.FootComment = m.Content[i+1].HeadComment, m.Content[i+1].LineComment, m.Content[i+1].FootComment
		m.Content[i].Value = name
		m.Content[i+1] = val
		return nil
	})
	if err == nil {
		fmt.Printf("\033[2J\033[H\nSaved to %s\n\n", path)
	}
	return err
}

func deleteConnection(path string, c conn) error {
	fmt.Printf("\nDelete %q? (y/N): ", c.Name)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "y") {
		fmt.Println()
		return nil
	}
	err := updateConns(path, func(m *yaml.Node) error {
		i := pairIndex(m, c.Name)
		if i < 0 {
			return fmt.Errorf("%s not found in config", c.Name)
		}
		m.Content = append(m.Content[:i], m.Content[i+2:]...)
		return nil
	})
	if err == nil {
		fmt.Printf("Deleted %q\n\n", c.Name)
	}
	return err
}

func hasName(conns []conn, name string) bool {
	for _, c := range conns {
		if c.Name == name {
			return true
		}
	}
	return false
}

func pairIndex(m *yaml.Node, key string) int {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return i
		}
	}
	return -1
}

// updateConns edits the YAML node tree instead of re-marshalling structs so existing comments survive.
func updateConns(path string, fn func(conns *yaml.Node) error) error {
	var doc yaml.Node
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return err
	}
	if doc.Kind == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	root := doc.Content[0]

	var conns *yaml.Node
	if i := pairIndex(root, "connections"); i >= 0 {
		conns = root.Content[i+1]
	} else {
		conns = &yaml.Node{Kind: yaml.MappingNode}
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: "connections"}, conns)
	}
	if conns.Kind != yaml.MappingNode {
		*conns = yaml.Node{Kind: yaml.MappingNode}
	}

	if err := fn(conns); err != nil {
		return err
	}

	var out strings.Builder
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(out.String()), 0o600)
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
	if c.Password != "" && (c.Sudo == nil || *c.Sudo) {
		// Prime sudo's timestamp in the same tty so later sudo calls in this session skip the prompt.
		quoted := "'" + strings.ReplaceAll(c.Password, "'", `'\''`) + "'"
		args = append(args, "-t", target,
			fmt.Sprintf(`printf '%%s\n' %s | sudo -S -v -p ''; exec "${SHELL:-sh}" -l`, quoted))
	} else {
		args = append(args, target)
	}

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
