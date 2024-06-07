package git

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type Repository struct {
	wd string
}

func NewRepository(path string) Repository {
	return Repository{wd: path}
}

type ChangedFiles struct {
	Added    []string
	Modified []string
	Deleted  []string
	Renamed  map[string]string
}

func (r Repository) git(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", r.wd}, args...)...)

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("running %v: %w", cmd, err)
	}

	return out, nil
}

func (r Repository) GetCurrentCommit(ctx context.Context) (string, error) {
	head, err := r.git(ctx, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(head)), nil
}

func (r Repository) ChangedFiles(ctx context.Context, rev string) (ChangedFiles, error) {
	cf := ChangedFiles{
		Renamed: make(map[string]string),
	}

	out, err := r.git(ctx, "diff", "--name-status", rev)
	if err != nil {
		return cf, err
	}

	for _, line := range toLines(out) {
		l := strings.Fields(line)
		switch l[0] {
		case "A":
			cf.Added = append(cf.Added, l[1])
		case "M":
			cf.Modified = append(cf.Modified, l[1])
		case "D":
			cf.Deleted = append(cf.Deleted, l[1])
		default:
			if l[0][0] == 'R' { // it's a rename
				cf.Renamed[l[1]] = l[2]
			}
			// TODO(inkel) ignore for now
		}
	}

	return cf, nil
}

func (r Repository) Contents(ctx context.Context, head, path string) ([]byte, error) {
	return r.git(ctx, "cat-file", "blob", head+":"+path)
}

func toLines(b []byte) []string {
	var lines []string

	s := bufio.NewScanner(bytes.NewBuffer(b))

	for s.Scan() {
		lines = append(lines, s.Text())
	}

	if err := s.Err(); err != nil {
		// TODO(inkel) panicking is probably not the best here
		panic(err)
	}

	return lines
}
