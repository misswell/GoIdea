package github

import (
	"fmt"
	"net/url"
	"strings"
)

// Coordinates identifies a repository on github.com.
type Coordinates struct {
	Owner string
	Name  string
}

func (c Coordinates) String() string {
	return c.Owner + "/" + c.Name
}

// RepositoryURL is the canonical browser URL of the repository.
func (c Coordinates) RepositoryURL() string {
	return "https://github.com/" + c.String()
}

// ParseRemote parses the GitHub remotes Git writes for HTTPS, SSH and the
// scp-like ssh form. Credentials, ports, query strings and fragments are
// rejected so a token is never sent to an address other than github.com.
func ParseRemote(raw string) (Coordinates, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Coordinates{}, fmt.Errorf("empty Git remote")
	}
	var host, path string
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return Coordinates{}, fmt.Errorf("invalid Git remote")
		}
		if u.User != nil && (u.User.Username() != "git" || passwordPresent(u.User)) {
			return Coordinates{}, fmt.Errorf("Git remote contains credentials")
		}
		if u.RawQuery != "" || u.Fragment != "" {
			return Coordinates{}, fmt.Errorf("Git remote has a query or fragment")
		}
		switch u.Scheme {
		case "https", "ssh", "git":
		default:
			return Coordinates{}, fmt.Errorf("unsupported Git remote scheme %q", u.Scheme)
		}
		if u.Port() != "" {
			return Coordinates{}, fmt.Errorf("GitHub remotes with a port are not supported")
		}
		host, path = strings.ToLower(u.Hostname()), u.Path
	} else {
		left, right, ok := strings.Cut(raw, ":")
		if !ok {
			return Coordinates{}, fmt.Errorf("unsupported Git remote")
		}
		if at := strings.LastIndex(left, "@"); at >= 0 {
			if left[:at] != "git" {
				return Coordinates{}, fmt.Errorf("Git remote contains credentials")
			}
			left = left[at+1:]
		}
		host, path = strings.ToLower(left), right
	}
	if host != "github.com" {
		return Coordinates{}, fmt.Errorf("the Git remote is not on github.com")
	}
	path = strings.Trim(strings.TrimSpace(path), "/")
	path = strings.TrimSuffix(path, ".git")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || !validRepositoryPart(parts[0]) || !validRepositoryPart(parts[1]) {
		return Coordinates{}, fmt.Errorf("GitHub remote must name owner/repository")
	}
	return Coordinates{Owner: parts[0], Name: parts[1]}, nil
}

func passwordPresent(u *url.Userinfo) bool {
	_, ok := u.Password()
	return ok
}

func validRepositoryPart(part string) bool {
	if part == "" || part == "." || part == ".." || len(part) > 100 {
		return false
	}
	for _, r := range part {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.':
		default:
			return false
		}
	}
	return true
}
