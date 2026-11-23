package doctor

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/alvaroofernaandez/aval/internal/core/config"
)

// checkConfig reports on aval's own config file: bad values and unknown keys
// warn with file:line, a missing file is simply "defaults".
func checkConfig(opt Options) Result {
	r := Result{ID: "config", Status: OK}
	if _, err := os.Stat(opt.ConfigPath); errors.Is(err, fs.ErrNotExist) {
		r.Message = "no config file, using defaults"
		return r
	}
	f, warns, err := config.Load(opt.ConfigPath)
	if err != nil {
		r.Status, r.Message = Warn, err.Error()
		return r
	}
	var problems []string
	for _, w := range warns {
		problems = append(problems, strings.TrimPrefix(w.String(), "config: "))
	}
	for _, u := range f.UnknownKeys() {
		problems = append(problems, fmt.Sprintf("%s:%d: unknown key %q. Valid: %s.",
			opt.ConfigPath, u.Line, u.Key, strings.Join(config.Keys(), ", ")))
	}
	if len(problems) > 0 {
		r.Status, r.Message, r.Details = Warn, strings.Join(problems, " | "), problems
		return r
	}
	r.Message = opt.ConfigPath + " is valid"
	return r
}
