package cmake

import "strings"

type token struct {
	text        string
	punctuation bool
}

// declarations reads literal arguments only: evaluating CMake variables would require a build.
func declarations(src string) []dependency {
	tokens := lex(src)
	var out []dependency
	for i := 0; i+1 < len(tokens); i++ {
		command := strings.ToLower(tokens[i].text)
		if tokens[i].punctuation || !tokens[i+1].punctuation || tokens[i+1].text != "(" {
			continue
		}
		start := i + 2
		level := 1
		end := start
		for ; end < len(tokens); end++ {
			if tokens[end].punctuation {
				switch tokens[end].text {
				case "(":
					level++
				case ")":
					level--
				}
			}
			if level == 0 {
				break
			}
		}
		i = end
		if level != 0 || (command != "fetchcontent_declare" && command != "cpmaddpackage") {
			continue
		}
		args := tokens[start:end]
		name, url := "", ""
		if command == "fetchcontent_declare" && len(args) > 0 {
			name = args[0].text
		}
		for j := 0; j+1 < len(args); j++ {
			switch strings.ToUpper(args[j].text) {
			case "NAME":
				if command == "cpmaddpackage" {
					name = args[j+1].text
				}
			case "GIT_REPOSITORY":
				url = args[j+1].text
			}
		}
		if validName(name) && url != "" && !strings.ContainsAny(url, "$;") {
			out = append(out, dependency{Name: name, URL: url})
		}
	}
	return out
}

func validName(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("_.+-", c) {
			continue
		}
		return false
	}
	return true
}

// bracket returns the content and end offset of a CMake [=[...]=] argument or comment.
func bracket(src string, start int) (string, int, bool) {
	if src[start] != '[' {
		return "", start, false
	}
	i := start + 1
	for i < len(src) && src[i] == '=' {
		i++
	}
	if i >= len(src) || src[i] != '[' {
		return "", start, false
	}
	closing := "]" + src[start+1:i] + "]"
	end := strings.Index(src[i+1:], closing)
	if end < 0 {
		return "", len(src), true
	}
	return src[i+1 : i+1+end], i + 1 + end + len(closing), true
}

func lex(src string) []token {
	var out []token
	for i := 0; i < len(src); {
		switch src[i] {
		case ' ', '\t', '\n', '\r':
			i++
			continue
		case '#':
			i++
			if i < len(src) {
				if _, end, ok := bracket(src, i); ok {
					i = end
					continue
				}
			}
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		case '(', ')':
			out = append(out, token{text: src[i : i+1], punctuation: true})
			i++
			continue
		case '[':
			if text, end, ok := bracket(src, i); ok {
				out = append(out, token{text: text})
				i = end
				continue
			}
		}
		quoted := src[i] == '"'
		if quoted {
			i++
		}
		var b strings.Builder
		for i < len(src) {
			c := src[i]
			if quoted && c == '"' {
				i++
				break
			}
			if !quoted && strings.ContainsRune(" \t\r\n()#", rune(c)) {
				break
			}
			if c == '\\' && i+1 < len(src) {
				i++
				c = src[i]
			}
			b.WriteByte(c)
			i++
		}
		out = append(out, token{text: b.String()})
	}
	return out
}
