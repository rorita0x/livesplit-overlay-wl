package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

type Setting struct {
	Name    string
	section string
	key     string
	value   reflect.Value
}

func (c *Config) Settings() []Setting {
	var list []Setting
	var walk func(v reflect.Value, section string)
	walk = func(v reflect.Value, section string) {
		for i := range v.NumField() {
			key := v.Type().Field(i).Tag.Get("toml")
			fv := v.Field(i)
			if fv.Kind() == reflect.Struct {
				walk(fv, key)
				continue
			}
			name := key
			if section != "" {
				name = section + "." + key
			}
			list = append(list, Setting{Name: name, section: section, key: key, value: fv})
		}
	}
	walk(reflect.ValueOf(c).Elem(), "")
	return list
}

func (c *Config) Setting(name string) (Setting, error) {
	var names []string
	for _, s := range c.Settings() {
		if s.Name == name {
			return s, nil
		}
		names = append(names, s.Name)
	}
	return Setting{}, fmt.Errorf("unknown setting %q, valid settings: %s", name, strings.Join(names, ", "))
}

func (s Setting) Value() any { return s.value.Interface() }

func (s Setting) TOML() string {
	v := s.value.Interface()
	if list, ok := v.([]string); ok && len(list) == 0 {
		return s.key + " = []"
	}
	var buf bytes.Buffer
	toml.NewEncoder(&buf).Encode(map[string]any{s.key: v})
	return strings.TrimSpace(buf.String())
}

func (s Setting) parse(input string) (any, error) {
	switch s.value.Kind() {
	case reflect.String:
		return input, nil
	case reflect.Int:
		return strconv.Atoi(input)
	case reflect.Float64:
		return strconv.ParseFloat(input, 64)
	case reflect.Bool:
		return strconv.ParseBool(input)
	case reflect.Slice:
		list := []string{}
		for item := range strings.SplitSeq(input, ",") {
			if item = strings.TrimSpace(item); item != "" {
				list = append(list, item)
			}
		}
		return list, nil
	}
	return nil, fmt.Errorf("%s: unsupported type %s", s.Name, s.value.Kind())
}

func (s Setting) hint() string {
	switch s.value.Kind() {
	case reflect.Int:
		return "a whole number"
	case reflect.Float64:
		return "a number like 0.8"
	case reflect.Bool:
		return "true or false"
	}
	return "a value"
}

func (s Setting) line(input string) (string, error) {
	v, err := s.parse(input)
	if err != nil {
		return "", fmt.Errorf("%s must be %s, got %q", s.Name, s.hint(), input)
	}
	if list, ok := v.([]string); ok && len(list) == 0 {
		return s.key + " = []", nil
	}
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(map[string]any{s.key: v}); err != nil {
		return "", err
	}
	return strings.TrimSpace(buf.String()), nil
}

var sectionHeader = regexp.MustCompile(`^\s*\[([^\]]*)\]`)

type document struct {
	lines []string
}

func readDocument(path string) (*document, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &document{}, nil
	}
	if err != nil {
		return nil, err
	}
	text := strings.TrimRight(string(data), "\n")
	if text == "" {
		return &document{}, nil
	}
	return &document{lines: strings.Split(text, "\n")}, nil
}

func (d *document) String() string {
	return strings.Join(d.lines, "\n") + "\n"
}

func (d *document) sectionRange(section string) (start, end int, found bool) {
	current, start := "", 0
	found = section == ""
	for i, l := range d.lines {
		m := sectionHeader.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		if current == section && found {
			return start, i, true
		}
		current = strings.TrimSpace(m[1])
		if current == section {
			start, found = i+1, true
		}
	}
	return start, len(d.lines), found
}

func (d *document) find(section, key string) (first, last int, ok bool) {
	start, end, found := d.sectionRange(section)
	if !found {
		return 0, 0, false
	}
	keyLine := regexp.MustCompile(`^\s*` + regexp.QuoteMeta(key) + `\s*=`)
	for i := start; i < end; i++ {
		if keyLine.MatchString(d.lines[i]) {
			last = i
			for depth := bracketDepth(d.lines[i]); depth > 0 && last+1 < end; {
				last++
				depth += bracketDepth(d.lines[last])
			}
			return i, last, true
		}
	}
	return 0, 0, false
}

func (d *document) set(section, key, line string) {
	if first, last, ok := d.find(section, key); ok {
		if comment := trailingComment(d.lines[last]); comment != "" {
			line += "  " + comment
		}
		d.lines = append(d.lines[:first], append([]string{line}, d.lines[last+1:]...)...)
		return
	}
	start, end, found := d.sectionRange(section)
	if !found {
		if len(d.lines) > 0 {
			d.lines = append(d.lines, "")
		}
		d.lines = append(d.lines, "["+section+"]", line)
		return
	}
	at := end
	for at > start && strings.TrimSpace(d.lines[at-1]) == "" {
		at--
	}
	d.lines = append(d.lines[:at], append([]string{line}, d.lines[at:]...)...)
}

func (d *document) unset(section, key string) bool {
	first, last, ok := d.find(section, key)
	if ok {
		d.lines = append(d.lines[:first], d.lines[last+1:]...)
	}
	return ok
}

func scanOutsideStrings(line string, visit func(i int, r rune) bool) {
	var quote rune
	escaped := false
	for i, r := range line {
		switch {
		case escaped:
			escaped = false
		case quote == '"' && r == '\\':
			escaped = true
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '"' || r == '\'':
			quote = r
		default:
			if !visit(i, r) {
				return
			}
		}
	}
}

func trailingComment(line string) string {
	comment := ""
	scanOutsideStrings(line, func(i int, r rune) bool {
		if r == '#' {
			comment = line[i:]
			return false
		}
		return true
	})
	return comment
}

func bracketDepth(line string) int {
	depth := 0
	scanOutsideStrings(line, func(_ int, r rune) bool {
		switch r {
		case '#':
			return false
		case '[':
			depth++
		case ']':
			depth--
		}
		return true
	})
	return depth
}

func Set(path, name, input string, validate func(Config) error) error {
	cfg := Default()
	s, err := cfg.Setting(name)
	if err != nil {
		return err
	}
	if (name == "splits" || name == "layout") && input != "" && !strings.HasPrefix(input, "~") {
		if input, err = filepath.Abs(input); err != nil {
			return err
		}
	}
	line, err := s.line(input)
	if err != nil {
		return err
	}
	return edit(path, validate, func(d *document) { d.set(s.section, s.key, line) })
}

func Unset(path, name string, validate func(Config) error) error {
	cfg := Default()
	s, err := cfg.Setting(name)
	if err != nil {
		return err
	}
	return edit(path, validate, func(d *document) { d.unset(s.section, s.key) })
}

func edit(path string, validate func(Config) error, change func(*document)) error {
	doc, err := readDocument(path)
	if err != nil {
		return err
	}
	change(doc)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(doc.String()), 0o644); err != nil {
		return err
	}
	cfg, err := Load(tmp, true)
	if err == nil && validate != nil {
		err = validate(cfg)
	}
	if err != nil {
		os.Remove(tmp)
		return errors.New(strings.Replace(err.Error(), "config "+tmp+": ", "", 1))
	}
	return os.Rename(tmp, path)
}
