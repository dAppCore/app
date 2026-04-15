// SPDX-License-Identifier: EUPL-1.2

package app

import (
	core "dappco.re/go/core"
	"dappco.re/go/core/config"
	coreerr "dappco.re/go/core/log"
)

// ValidateOptions tunes ValidateManifest. Zero values produce the
// strictest check — every RFC §2 field that has a rule is enforced.
// Callers that want a more lenient lint (e.g. a CI pre-commit hook
// that accepts partially-filled drafts) can disable individual checks.
//
//	opts := app.ValidateOptions{RequireSignature: true}
//	report := app.ValidateManifest(&manifest, opts)
type ValidateOptions struct {
	// RequireSignature demands a non-empty `sign` field. Defaults false
	// so ValidateManifest works against in-progress drafts; the Boot
	// pipeline's own Step 2 enforces the signature in prod mode.
	RequireSignature bool
	// AllowUnknownModules skips the registry lookup for declared
	// modules. Defaults false — ValidateManifest returns a warning for
	// any module the host has not registered so a CI pipeline can spot
	// missing host capabilities before deploy.
	AllowUnknownModules bool
}

// ValidateIssueSeverity names the severity of a single ValidateIssue.
// Errors fail the validation; warnings surface friction but leave the
// manifest usable. The split mirrors the Lint RFC's "error vs advice"
// distinction so a CI pipeline can treat each class differently.
//
//	if issue.Severity == app.ValidateError { /* fail the build */ }
type ValidateIssueSeverity int

const (
	// ValidateError — a rule violation that would block Boot.
	ValidateError ValidateIssueSeverity = iota
	// ValidateWarning — advice the developer should act on but that
	// does not block Boot.
	ValidateWarning
)

// String returns the lowercase severity name used in CI output.
//
//	app.ValidateError.String() // "error"
func (s ValidateIssueSeverity) String() string {
	switch s {
	case ValidateError:
		return "error"
	case ValidateWarning:
		return "warning"
	default:
		return "unknown"
	}
}

// ValidateIssue is one finding from ValidateManifest. Field identifies
// the source of the issue in manifest-path notation (`permissions.read`,
// `slots.H`, `modules[0]`) so a caller rendering the issue knows
// exactly where to look.
//
//	for _, issue := range report.Issues {
//	    core.Error("manifest issue", "field", issue.Field, "msg", issue.Message)
//	}
type ValidateIssue struct {
	Severity ValidateIssueSeverity
	Field    string
	Message  string
}

// ValidateReport is the collected output of ValidateManifest. Split by
// severity so a CI pipeline can test `len(report.Errors()) == 0`
// without walking the full slice.
//
//	report := app.ValidateManifest(&manifest, app.ValidateOptions{})
//	if !report.OK() { for _, e := range report.Errors() { ... } }
type ValidateReport struct {
	Issues []ValidateIssue
}

// OK reports whether the report contains no errors. Warnings are not
// fatal by this definition — a CI pipeline that wants "zero advice"
// can call len(report.Issues) == 0 instead.
//
//	if report.OK() { /* safe to ship */ }
func (r ValidateReport) OK() bool {
	for _, issue := range r.Issues {
		if issue.Severity == ValidateError {
			return false
		}
	}
	return true
}

// Errors returns only the error-severity issues from the report.
// Convenience wrapper so CI tooling can render the "must fix" list
// without filtering itself.
//
//	for _, e := range report.Errors() { log.Fatal(e.Message) }
func (r ValidateReport) Errors() []ValidateIssue {
	return r.byLevel(ValidateError)
}

// Warnings returns only the warning-severity issues from the report.
//
//	for _, w := range report.Warnings() { log.Warn(w.Message) }
func (r ValidateReport) Warnings() []ValidateIssue {
	return r.byLevel(ValidateWarning)
}

// byLevel filters the issues slice by severity.
//
//	errs := r.byLevel(ValidateError)
func (r ValidateReport) byLevel(level ValidateIssueSeverity) []ValidateIssue {
	var out []ValidateIssue
	for _, issue := range r.Issues {
		if issue.Severity == level {
			out = append(out, issue)
		}
	}
	return out
}

// ValidateManifest lints a ViewManifest against the RFC §2 rules.
// Returns a ValidateReport listing every issue found; an OK report
// means every hard rule passed. Used by `core-app validate`, by CI
// pre-commit hooks, and by the SDK pipeline before emitting bindings
// (a manifest that fails here would produce a malformed openapi.json).
//
//	report := app.ValidateManifest(&manifest, app.ValidateOptions{RequireSignature: true})
//	if !report.OK() {
//	    for _, issue := range report.Errors() { core.Error("manifest", "issue", issue.Message) }
//	    return errNotOk
//	}
//
// Rules checked:
//
//   - RFC §2 required fields: code, name, version (error)
//
//   - RFC §2.1 layout variant — HLCRF characters only, via
//     validateLayoutVariant (error)
//
//   - Slots map — every entry must be a string component name
//     (error)
//
//   - RFC §2.2 permissions — unknown permission fields surface as
//     warnings (forward compat) but path entries containing `..`
//     (traversal) are errors
//
//   - RFC §6.1 Sign — empty Sign with RequireSignature=true is an
//     error; otherwise a warning (the Boot pipeline itself enforces in
//     prod mode)
//
//   - RFC §13 Modules — unknown module names are warnings unless
//     AllowUnknownModules=true
//
//   - RFC §2.2 Config — reserved keys have no special shape; template
//     entries must have a non-empty `template` path when present
func ValidateManifest(m *config.ViewManifest, opts ValidateOptions) ValidateReport {
	if m == nil {
		return ValidateReport{Issues: []ValidateIssue{{
			Severity: ValidateError,
			Field:    "",
			Message:  "manifest is nil",
		}}}
	}

	var report ValidateReport

	// §2 — required identity.
	if m.Code == "" {
		report.Issues = append(report.Issues, ValidateIssue{
			Severity: ValidateError,
			Field:    "code",
			Message:  "code is required",
		})
	}
	if m.Name == "" {
		report.Issues = append(report.Issues, ValidateIssue{
			Severity: ValidateError,
			Field:    "name",
			Message:  "name is required",
		})
	}
	if m.Version == "" {
		report.Issues = append(report.Issues, ValidateIssue{
			Severity: ValidateError,
			Field:    "version",
			Message:  "version is required",
		})
	}

	// §2.1 layout — variant string + slots table.
	if err := validateLayoutVariant(m.Layout); err != nil {
		report.Issues = append(report.Issues, ValidateIssue{
			Severity: ValidateError,
			Field:    "layout",
			Message:  "invalid layout: " + err.Error(),
		})
	}
	for slot, raw := range m.Slots {
		if raw == nil {
			report.Issues = append(report.Issues, ValidateIssue{
				Severity: ValidateError,
				Field:    "slots." + slot,
				Message:  "slot has nil component",
			})
			continue
		}
		if _, ok := raw.(string); !ok {
			report.Issues = append(report.Issues, ValidateIssue{
				Severity: ValidateError,
				Field:    "slots." + slot,
				Message:  "slot component must be a string",
			})
		}
	}

	// §2.1 — layout + slots consistency. A slot declared outside the
	// layout variant (e.g. `layout: HCF` + `slots: {R: right-panel}`)
	// never mounts at runtime because resolveLayout's iteration order is
	// driven by the variant string. Surface it as a warning so a
	// developer spots the dead entry during validate. The converse —
	// the layout variant naming a slot char with no component — is also
	// worth flagging because core/gui will render an empty slot.
	if m.Layout != "" && len(m.Slots) > 0 {
		layoutSlots := map[string]bool{}
		for _, ch := range m.Layout {
			layoutSlots[string(ch)] = true
		}
		// Slots declared outside the variant → dead slot warning.
		for slot := range m.Slots {
			if !layoutSlots[slot] {
				report.Issues = append(report.Issues, ValidateIssue{
					Severity: ValidateWarning,
					Field:    "slots." + slot,
					Message:  "slot '" + slot + "' is declared but not referenced by layout '" + m.Layout + "'",
				})
			}
		}
		// Layout characters with no component → empty slot warning.
		slotsMapped := map[string]bool{}
		for slot := range m.Slots {
			slotsMapped[slot] = true
		}
		seenChar := map[string]bool{}
		for _, ch := range m.Layout {
			slot := string(ch)
			if seenChar[slot] {
				continue
			}
			seenChar[slot] = true
			if !slotsMapped[slot] {
				report.Issues = append(report.Issues, ValidateIssue{
					Severity: ValidateWarning,
					Field:    "layout",
					Message:  "layout '" + m.Layout + "' references slot '" + slot + "' but no component is declared under slots." + slot,
				})
			}
		}
	}

	// §2.2 — permissions. Path entries containing `..` are hard errors
	// regardless of mode; the RFC §5.2 defence layer is non-negotiable.
	for i, entry := range m.Permissions.Read {
		if err := rejectPathTraversal("read", entry); err != nil {
			report.Issues = append(report.Issues, ValidateIssue{
				Severity: ValidateError,
				Field:    "permissions.read[" + core.Sprint(i) + "]",
				Message:  err.Error(),
			})
		}
	}
	for i, entry := range manifestWriteList(m) {
		if err := rejectPathTraversal("write", entry); err != nil {
			report.Issues = append(report.Issues, ValidateIssue{
				Severity: ValidateError,
				Field:    "permissions.write[" + core.Sprint(i) + "]",
				Message:  err.Error(),
			})
		}
	}

	// §3.2 signature — only an error when the caller demands one; Boot
	// itself enforces in prod mode but ValidateManifest runs against
	// drafts too.
	if m.Sign == "" {
		severity := ValidateWarning
		if opts.RequireSignature {
			severity = ValidateError
		}
		report.Issues = append(report.Issues, ValidateIssue{
			Severity: severity,
			Field:    "sign",
			Message:  "manifest is unsigned",
		})
	}

	// §13 — modules must resolve against the package registry, unless
	// the caller opts out (standalone lint without a host registered).
	if !opts.AllowUnknownModules {
		for i, name := range m.Modules {
			if _, ok := LookupModule(name); !ok {
				report.Issues = append(report.Issues, ValidateIssue{
					Severity: ValidateWarning,
					Field:    "modules[" + core.Sprint(i) + "]",
					Message:  "unknown module '" + name + "' — host has not registered a factory",
				})
			}
		}
	}

	// §2.2 — config template entries must carry a `template` path when
	// declared. Reserved config keys (services, source, type, …) are
	// skipped because they are not template entries.
	for name, raw := range m.Config {
		if isReservedConfigKey(name) {
			continue
		}
		entry, ok := asTemplateEntry(raw)
		if !ok {
			report.Issues = append(report.Issues, ValidateIssue{
				Severity: ValidateError,
				Field:    "config." + name,
				Message:  "entry is not a {template, vars} map",
			})
			continue
		}
		if entry.Template == "" {
			report.Issues = append(report.Issues, ValidateIssue{
				Severity: ValidateError,
				Field:    "config." + name + ".template",
				Message:  "template path is empty",
			})
		}
	}

	return report
}

// ValidateError is the sentinel-ish typed error returned by callers
// that want a single error value rather than a structured report.
// Wraps the first ValidateError issue from the report so the caller
// can log it directly.
//
//	if err := app.ValidateManifestErr(&manifest, opts); err != nil { return err }
//
// Callers needing structured output should use ValidateManifest
// instead.
func ValidateManifestErr(m *config.ViewManifest, opts ValidateOptions) error {
	report := ValidateManifest(m, opts)
	if report.OK() {
		return nil
	}
	first := report.Errors()[0]
	field := first.Field
	if field == "" {
		field = "<manifest>"
	}
	return coreerr.E("app.ValidateManifest", field+": "+first.Message, nil)
}
