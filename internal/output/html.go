package output

import (
	"fmt"
	"html/template"
	"io"
	"sort"
	"time"

	"github.com/kaeawc/datalint/internal/diag"
)

// WriteHTML renders findings as a self-contained HTML page: a summary
// table at the top, then one section per rule listing each finding's
// location and message. now is injected so callers (tests) can pin
// the timestamp.
func WriteHTML(w io.Writer, findings []diag.Finding, version string, now time.Time) error {
	view := buildHTMLView(findings, version, now)
	return htmlTemplate.Execute(w, view)
}

type htmlView struct {
	Version       string
	Generated     string
	TotalFindings int
	Groups        []htmlGroup
}

type htmlGroup struct {
	RuleID        string
	SeverityCSS   string
	SeverityLabel string
	Count         int
	Findings      []htmlFinding
}

type htmlFinding struct {
	LocationDisplay string
	Message         string
}

func buildHTMLView(findings []diag.Finding, version string, now time.Time) htmlView {
	byRule := map[string][]diag.Finding{}
	for _, f := range findings {
		byRule[f.RuleID] = append(byRule[f.RuleID], f)
	}
	ruleIDs := make([]string, 0, len(byRule))
	for id := range byRule {
		ruleIDs = append(ruleIDs, id)
	}
	sort.Strings(ruleIDs)

	groups := make([]htmlGroup, 0, len(ruleIDs))
	for _, id := range ruleIDs {
		rule := byRule[id]
		groups = append(groups, htmlGroup{
			RuleID:        id,
			SeverityCSS:   htmlSeverityCSS(rule[0].Severity),
			SeverityLabel: htmlSeverityLabel(rule[0].Severity),
			Count:         len(rule),
			Findings:      toHTMLFindings(rule),
		})
	}
	return htmlView{
		Version:       version,
		Generated:     now.UTC().Format(time.RFC3339),
		TotalFindings: len(findings),
		Groups:        groups,
	}
}

func toHTMLFindings(findings []diag.Finding) []htmlFinding {
	out := make([]htmlFinding, 0, len(findings))
	for _, f := range findings {
		out = append(out, htmlFinding{
			LocationDisplay: locationDisplay(f.Location),
			Message:         f.Message,
		})
	}
	return out
}

func locationDisplay(loc diag.Location) string {
	if loc.Path == "" {
		return "(no location)"
	}
	line := loc.Line
	if line == 0 && loc.Row != 0 {
		line = loc.Row
	}
	if line > 0 {
		return fmt.Sprintf("%s:%d", loc.Path, line)
	}
	return loc.Path
}

func htmlSeverityCSS(s diag.Severity) string {
	switch s {
	case diag.SeverityError:
		return "error"
	case diag.SeverityWarning:
		return "warning"
	case diag.SeverityInfo:
		return "info"
	}
	return "info"
}

func htmlSeverityLabel(s diag.Severity) string {
	switch s {
	case diag.SeverityError:
		return "error"
	case diag.SeverityWarning:
		return "warning"
	case diag.SeverityInfo:
		return "info"
	}
	return "info"
}

const htmlSource = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>datalint report</title>
<style>
  body { font: 14px/1.5 -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif; color: #1f2328; max-width: 960px; margin: 24px auto; padding: 0 16px; }
  header { border-bottom: 1px solid #d0d7de; padding-bottom: 16px; margin-bottom: 16px; }
  h1 { font-size: 20px; margin: 0 0 4px; }
  header p { margin: 2px 0; color: #57606a; }
  table { border-collapse: collapse; width: 100%; margin-bottom: 24px; }
  th, td { text-align: left; padding: 6px 10px; border-bottom: 1px solid #d0d7de; }
  th { background: #f6f8fa; font-weight: 600; }
  td a { color: #0969da; text-decoration: none; }
  td a:hover { text-decoration: underline; }
  section.rule { margin-bottom: 24px; }
  section.rule h2 { font-size: 16px; margin: 0 0 8px; }
  ol { margin: 0; padding-left: 20px; }
  li { margin-bottom: 4px; }
  code { font-family: ui-monospace, SFMono-Regular, "SF Mono", monospace; background: #f6f8fa; padding: 1px 6px; border-radius: 4px; font-size: 12px; }
  .badge { display: inline-block; padding: 1px 8px; border-radius: 10px; font-size: 11px; font-weight: 600; text-transform: uppercase; vertical-align: middle; margin-left: 6px; }
  .sev-error { background: #ffebe9; color: #82071e; }
  .sev-warning { background: #fff8c5; color: #7d4e00; }
  .sev-info { background: #ddf4ff; color: #0a3069; }
  .empty { color: #57606a; font-style: italic; }
</style>
</head>
<body>
<header>
  <h1>datalint report</h1>
  <p>version {{ .Version }} · generated {{ .Generated }}</p>
  <p>{{ .TotalFindings }} findings across {{ len .Groups }} rules</p>
</header>
{{ if not .Groups }}
<p class="empty">No findings.</p>
{{ else }}
<table>
  <thead><tr><th>Rule</th><th>Severity</th><th>Count</th></tr></thead>
  <tbody>
  {{ range .Groups }}
    <tr>
      <td><a href="#{{ .RuleID }}">{{ .RuleID }}</a></td>
      <td><span class="badge sev-{{ .SeverityCSS }}">{{ .SeverityLabel }}</span></td>
      <td>{{ .Count }}</td>
    </tr>
  {{ end }}
  </tbody>
</table>
{{ range .Groups }}
<section class="rule" id="{{ .RuleID }}">
  <h2>{{ .RuleID }} <span class="badge sev-{{ .SeverityCSS }}">{{ .SeverityLabel }}</span></h2>
  <ol>
  {{ range .Findings }}
    <li><code>{{ .LocationDisplay }}</code> &mdash; {{ .Message }}</li>
  {{ end }}
  </ol>
</section>
{{ end }}
{{ end }}
</body>
</html>
`

var htmlTemplate = template.Must(template.New("report").Parse(htmlSource))
