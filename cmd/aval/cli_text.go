package main

import (
	"strings"

	"github.com/alvaroofernaandez/aval/internal/core/doctor"
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
