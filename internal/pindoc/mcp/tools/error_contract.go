package tools

import (
	"reflect"
	"regexp"
	"strings"
)

// ErrorChecklistItem is the locale-safe pair clients should display from
// not_ready responses. Code is the stable branch key; Message is already
// localized by the tool using MessageLocale/fallback rules.
type ErrorChecklistItem struct {
	Code    string `json:"code" jsonschema:"stable SCREAMING_SNAKE_CASE identifier; branch on this, not message text"`
	Message string `json:"message" jsonschema:"localized user-facing checklist copy"`
}

var stableErrorCodeRE = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

func applyMCPErrorContract[O any](output O, lang string) O {
	rv := reflect.ValueOf(&output).Elem()
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return output
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return output
	}

	status := stringField(rv, "Status")
	errorCode := stringField(rv, "ErrorCode")
	failed := stringSliceField(rv, "Failed")
	checklist := stringSliceField(rv, "Checklist")
	if status != "not_ready" || (errorCode == "" && len(failed) == 0 && len(checklist) == 0) {
		return output
	}

	codes := stableMCPErrorCodes(errorCode, failed)
	if f := rv.FieldByName("Failed"); f.IsValid() && f.CanSet() && f.Kind() == reflect.Slice && len(codes) > 0 {
		f.Set(reflect.ValueOf(codes))
	}
	if f := rv.FieldByName("ErrorCodes"); f.IsValid() && f.CanSet() && f.Kind() == reflect.Slice && f.Len() == 0 {
		if len(codes) > 0 {
			f.Set(reflect.ValueOf(codes))
		}
	}
	if f := rv.FieldByName("ChecklistItems"); f.IsValid() && f.CanSet() && f.Kind() == reflect.Slice && f.Len() == 0 {
		items := mcpChecklistItems(errorCode, failed, checklist)
		if len(items) > 0 {
			v := reflect.ValueOf(items)
			if v.Type().AssignableTo(f.Type()) {
				f.Set(v)
			}
		}
	}
	if f := rv.FieldByName("MessageLocale"); f.IsValid() && f.CanSet() && f.Kind() == reflect.String && f.String() == "" {
		f.SetString(normalizeMessageLocale(lang))
	}
	return output
}

func stableMCPErrorCodes(errorCode string, failed []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 1+len(failed))
	push := func(raw string) {
		code := stableMCPErrorCode(raw)
		if code == "" {
			return
		}
		if _, ok := seen[code]; ok {
			return
		}
		seen[code] = struct{}{}
		out = append(out, code)
	}
	push(errorCode)
	for _, code := range failed {
		push(code)
	}
	return out
}

func mcpChecklistItems(errorCode string, failed, checklist []string) []ErrorChecklistItem {
	if len(checklist) == 0 {
		return nil
	}
	out := make([]ErrorChecklistItem, 0, len(checklist))
	for i, message := range checklist {
		message = strings.TrimSpace(message)
		if message == "" {
			continue
		}
		rawCode := errorCode
		if i < len(failed) && strings.TrimSpace(failed[i]) != "" {
			rawCode = failed[i]
		}
		code := stableMCPErrorCode(rawCode)
		if code == "" {
			code = "UNKNOWN_ERROR"
		}
		out = append(out, ErrorChecklistItem{Code: code, Message: message})
	}
	return out
}

func stableMCPErrorCode(raw string) string {
	code := strings.TrimSpace(raw)
	if code == "" {
		return ""
	}
	if i := strings.IndexByte(code, ':'); i > 0 {
		code = code[:i]
	}
	code = strings.ToUpper(code)
	var b strings.Builder
	underscore := false
	for _, r := range code {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
			underscore = false
		default:
			if !underscore {
				b.WriteByte('_')
				underscore = true
			}
		}
	}
	code = strings.Trim(b.String(), "_")
	if code == "" {
		return ""
	}
	if stableErrorCodeRE.MatchString(code) {
		return code
	}
	return ""
}

func normalizeMessageLocale(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if lang == "" || lang == "auto" {
		return "en"
	}
	if i := strings.IndexAny(lang, "-_"); i > 0 {
		lang = lang[:i]
	}
	switch lang {
	case "ko", "ja", "en":
		return lang
	default:
		return "en"
	}
}

func stringField(rv reflect.Value, name string) string {
	f := rv.FieldByName(name)
	if !f.IsValid() || f.Kind() != reflect.String {
		return ""
	}
	return f.String()
}

func stringSliceField(rv reflect.Value, name string) []string {
	f := rv.FieldByName(name)
	if !f.IsValid() || f.Kind() != reflect.Slice || f.Type().Elem().Kind() != reflect.String {
		return nil
	}
	out := make([]string, 0, f.Len())
	for i := 0; i < f.Len(); i++ {
		out = append(out, f.Index(i).String())
	}
	return out
}
