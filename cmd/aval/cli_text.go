package main

import (
	"errors"
	"strings"

	"github.com/alvaroofernaandez/aval/internal/core/doctor"
	"github.com/alvaroofernaandez/aval/internal/core/install"
	"github.com/alvaroofernaandez/aval/internal/i18n"
)

// The wording the commands print for people lives in the catalog
// (internal/i18n); this file adapts it to the packages that cannot import it.

// doctorText is the human table's wording in tr's language.
func doctorText(tr i18n.Translator) doctor.Text {
	return doctor.Text{
		Status:  tr.T("cli.doctor.col.status", nil),
		Check:   tr.T("cli.doctor.col.check", nil),
		Message: tr.T("cli.doctor.col.message", nil),
		Word: func(s doctor.Status) string {
			switch s {
			case doctor.OK:
				return tr.T("cli.doctor.status.ok", nil)
			case doctor.Warn:
				return tr.T("cli.doctor.status.warn", nil)
			case doctor.Fail:
				return tr.T("cli.doctor.status.fail", nil)
			}
			return strings.ToUpper(string(s))
		},
		Fixable: " " + tr.T("cli.doctor.fixable", nil),
		Summary: func(ok, warn, fail int) string {
			return tr.N("cli.doctor.count.ok", ok, nil) + ", " +
				tr.N("cli.doctor.count.warn", warn, nil) + ", " +
				tr.N("cli.doctor.count.fail", fail, nil)
		},
	}
}

// statusWord is what happened to one install action, in tr's language.
func statusWord(tr i18n.Translator, s install.Status) string {
	switch s {
	case install.StatusLinked:
		return tr.T("cli.install.status.linked", nil)
	case install.StatusCopied:
		return tr.T("cli.install.status.copied", nil)
	case install.StatusAlready:
		return tr.T("cli.install.status.already", nil)
	case install.StatusWould:
		return tr.T("cli.install.status.would", nil)
	}
	return s.String()
}

// localizedError is an error whose text a person reads in their language. It
// keeps the identity of the error it wraps: errors.Is and errors.As see
// through it, so only the rendering changes.
type localizedError struct {
	err  error
	text string
}

func (e localizedError) Error() string { return e.text }
func (e localizedError) Unwrap() error { return e.err }

// localizeErr renders the errors the install and update paths return in tr's
// language. Any other error is returned as it is.
func localizeErr(tr i18n.Translator, err error) error {
	var n actionsFailed
	switch {
	case errors.As(err, &n) && err == error(n):
		return localizedError{err: err, text: tr.N("cli.install.failed_count", int(n), nil)}
	case errors.Is(err, errNeedsTerminal):
		return localizedError{err: err, text: tr.T("cli.install.needs_terminal", nil)}
	}
	return err
}
