package costmodel

import (
	_ "embed"
	"fmt"
	"sort"
	"text/template"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// resourcesCost contains the detailed cost for cluster resources.
type resourcesCost struct {
	CPU       float64
	Memory    float64
	Storage   float64
	Kind      string
	Namespace string
	Name      string
}

func (c resourcesCost) Total() float64 {
	return c.CPU + c.Memory + c.Storage
}

func resourcesCosts(m *CostModel, req Requirements) resourcesCost {
	return resourcesCost{
		CPU:       m.CPU.NonSpotCPUForPeriod(Monthly, req.CPU),
		Memory:    m.RAM.NonSpotMemoryForPeriod(Monthly, req.Memory),
		Storage:   m.PersistentVolume.DollarsForPeriod(Monthly, req.PersistentVolume),
		Kind:      req.Kind,
		Namespace: req.Namespace,
		Name:      req.Name,
	}
}

// costReport holds information about a cluster & resource cost from
// its previous state and the desired new requirements.
type costReport struct {
	Cluster string

	Old, New resourcesCost
}

func (r costReport) Delta() float64 {
	return r.New.Total() - r.Old.Total()
}

func (s summaryReport) Delta() float64 {
	return s.New - s.Old
}

// costReports is a collection of CostReport.
type costReports []costReport
type summaryReport struct {
	Old, New float64
}

func (rs costReports) Totals() (float64, float64) {
	var n, o float64

	for _, r := range rs {
		n += r.New.Total()
		o += r.Old.Total()
	}

	return n, o
}

func (rs costReports) Sort() {
	sort.Slice(rs, func(i, j int) bool {
		// Higher deltas go on top
		return rs[i].Delta() > rs[j].Delta()
	})
}

// templateData holds the information passed to the markdown comment
// template.
type templateData struct {
	Reports map[string]costReports
	Summary map[string]summaryReport
	// Errors are events that aren't expected and can lead to unexpected results of kost.
	Errors []string
	// Warnings are expected events, or known limitations.
	Warnings []string
}

func (d templateData) Delta() float64 {
	var n, o float64
	for _, s := range d.Reports {
		nt, ot := s.Totals()
		n += nt
		o += ot

	}
	return n - o
}

func (d templateData) OldTotal() float64 {
	var output float64
	for _, s := range d.Reports {
		_, o := s.Totals()
		output += o
	}
	return output
}

// CommentPrefix is used to identify and hide old messages on GitHub.
const CommentPrefix = "<!-- kost -->"

// printer is a localized printer.
var printer = message.NewPrinter(language.English)

//go:embed comment.tmpl.md
var commentTemplate string

// templateFuncs holds the custom functions used within the template.
var templateFuncs = template.FuncMap{
	"commentPrefix": func() string { return CommentPrefix },
	"dollars": func(v float64) string {
		return displayCostInDollars(v)
	},
	"percentage": func(r float64) string {
		return fmt.Sprintf("%.2f%%", r*100)
	},
	"ratio": func(a, b float64) float64 {
		if b == 0 {
			if a > 0 {
				return 1
			}
			return 0
		}
		return a / b
	},
	"multiply": func(a, b float64) float64 {
		return a * b
	},
}

var tpl = template.Must(template.New("").Funcs(templateFuncs).Parse(commentTemplate))

// writeMarkdown
func (r *Reporter) writeMarkdown() error {
	d := templateData{
		Reports: make(map[string]costReports),
		Summary: make(map[string]summaryReport),
	}

	for _, r := range r.reports {
		if r.CostModel == nil {
			d.Errors = append(d.Errors, fmt.Sprintf("<code class=\"notranslate\">%v</code> report is missing cost model", r.To.Name))
			continue
		}

		// We have no good estimation for the time a (Cron)Job is running, therefor a cost estimation is highly impossible
		if r.To.Kind == "CronJob" || r.To.Kind == "Job" {
			d.Warnings = append(d.Warnings, fmt.Sprintf("<code class=\"notranslate\">%v</code> is a Job or CronJob, cost estimation impossible.", r.To.Name))
			continue
		}

		cr := costReport{
			Cluster: r.CostModel.Cluster.Name,
			Old:     resourcesCosts(r.CostModel, r.From),
			New:     resourcesCosts(r.CostModel, r.To),
		}
		reports := d.Reports[r.CostModel.Cluster.Name]
		reports = append(reports, cr)
		d.Reports[r.CostModel.Cluster.Name] = reports

		sr := d.Summary[r.CostModel.Cluster.Name]
		sr.Old += cr.Old.Total()
		sr.New += cr.New.Total()
		d.Summary[r.CostModel.Cluster.Name] = sr
	}

	for _, reports := range d.Reports {
		reports.Sort()
	}

	return tpl.Execute(r.Writer, d)
}
