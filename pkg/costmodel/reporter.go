package costmodel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

var (
	headers = []string{"Cluster", "Total Weekly Cost", "Δ Weekly Cost", "Total Monthly Cost", "Δ Monthly Cost"}
	periods = []Period{
		Weekly,
		Monthly,
	}
)

var ErrNoReports = errors.New("nothing to report")

type ReportType string

const (
	Table    ReportType = "table"
	Summary  ReportType = "summary"
	Markdown ReportType = "markdown"
)

type Reporter struct {
	Writer     io.Writer
	reports    []report
	reportType ReportType
	warnings   []string
	errors     []string
}

// report is a model for a cost report.
type report struct {
	CostModel *CostModel
	From      Requirements
	To        Requirements
}

// AddReport adds a costmodel and associated from, to resources to the reporter.
func (r *Reporter) AddReport(costModel *CostModel, from, to Requirements) {
	if to.Kind == "Job" || to.Kind == "Cronjob" {
		return
	}
	r.reports = append(r.reports, report{
		CostModel: costModel,
		From:      from,
		To:        to,
	})
}

// AddWarning records a non-fatal message that will be surfaced in the markdown
// report under the Warnings section. Use for expected events or known limitations
// (e.g., observed-replicas substitution applied for an HPA-managed workload).
func (r *Reporter) AddWarning(msg string) {
	r.warnings = append(r.warnings, msg)
}

// AddReportWithResolvedReplicas adds a cost report after substituting the manifest
// replica count with the observed 7d-average for HPA-managed Deployment/StatefulSet
// workloads. For unsupported kinds (DaemonSet, Pod, Job, CronJob) or when the
// CostModel cannot be used to identify a cluster, falls back to AddReport with the
// manifest replica count.
//
// On substitution, an audit-trail Warning is added to the reporter so reviewers can
// see that observed replicas were used. On resolver errors, the manifest count is
// preserved and an Error is added so misleading numbers are flagged rather than
// silently presented.
func (r *Reporter) AddReportWithResolvedReplicas(
	ctx context.Context,
	resolver HPAResolver,
	cm *CostModel,
	from, to Requirements,
) {
	id := to
	if id.Kind == "" {
		id = from
	}
	if cm == nil || cm.Cluster == nil || (id.Kind != "Deployment" && id.Kind != "StatefulSet") {
		r.AddReport(cm, from, to)
		return
	}

	replicas, source, err := ResolveReplicas(ctx, resolver, cm.Cluster.Name, id.Namespace, id.Kind, id.Name, id.Replicas)
	if err != nil {
		r.AddError(fmt.Sprintf("resolving replicas for %s/%s/%s on %s: %v",
			id.Namespace, id.Kind, id.Name, cm.Cluster.Name, err))
		r.AddReport(cm, from, to)
		return
	}

	if source == SourceObservedHPA {
		manifestReplicas := id.Replicas
		if from.Kind != "" {
			from.Replicas = replicas
		}
		if to.Kind != "" {
			to.Replicas = replicas
		}
		r.AddWarning(fmt.Sprintf(
			"used observed %d replicas (manifest: %d) for %s/%s/%s on %s — workload is HPA-managed",
			replicas, manifestReplicas, id.Namespace, id.Kind, id.Name, cm.Cluster.Name,
		))
	}

	r.AddReport(cm, from, to)
}

// AddError records a message about an unexpected event that may have led to
// inaccurate cost numbers. Surfaced under the Errors section in the markdown report.
func (r *Reporter) AddError(msg string) {
	r.errors = append(r.errors, msg)
}

func New(w io.Writer, reportType string) *Reporter {
	// If the writer passed in is nil, set it to io.Discard to prevent nil pointer exceptions later on
	if w == nil {
		w = io.Discard
	}
	return &Reporter{
		Writer:     w,
		reportType: ReportType(reportType),
	}
}

func (r *Reporter) Write() error {
	if len(r.reports) == 0 {
		return ErrNoReports
	}

	switch r.reportType {
	case Summary:
		return r.writeSummary()
	case Table:
		return r.writeTable()
	case Markdown:
		return r.writeMarkdown()
	default:
		return fmt.Errorf("report type %s not supported", r.reportType)
	}
}

func (r *Reporter) writeSummary() error {
	tabwriter := tabwriter.NewWriter(r.Writer, 8, 6, 2, ' ', 0)

	var p Period = Monthly
	fromTotalCost, toTotalCost := 0.0, 0.0
	for _, m := range r.reports {
		// Prevent a nil pointer exception here. Probably better ways to handle this
		if m.CostModel == nil {
			continue
		}
		from, to := calculateTotalCostForPeriod(p, m.From, m.To, m.CostModel)
		fromTotalCost += from
		toTotalCost += to
	}

	totalDiff := toTotalCost - fromTotalCost
	var rows []string
	rows = append(
		rows,
		fmt.Sprintf("PR changed the overall cost by %s(%.1f%%).", displayCostInDollars(totalDiff), percentageChange(fromTotalCost, toTotalCost)),
		fmt.Sprintf("Total Monthly Cost went from $%.2f to $%.2f.", fromTotalCost, toTotalCost),
	)
	if _, err := fmt.Fprintln(r.Writer, strings.Join(rows, "\n")); err != nil {
		return err
	}
	return tabwriter.Flush()
}

func (r *Reporter) writeTable() error {
	tabWriter := tabwriter.NewWriter(r.Writer, 8, 6, 2, ' ', 0)
	if _, err := fmt.Fprintln(tabWriter, strings.Join(headers, "\t")); err != nil {
		return err
	}
	totalCosts := make(map[string]float64)

	for _, m := range r.reports {
		row := []string{
			m.CostModel.Cluster.Name,
		}

		for _, p := range periods {
			keys := p.Keys()

			fromCost, toCost := calculateTotalCostForPeriod(p, m.From, m.To, m.CostModel)
			row = append(row,
				fmt.Sprintf("$%.2f", toCost),
				fmt.Sprintf("%s(%.1f%%)", displayCostInDollars(toCost-fromCost), percentageChange(fromCost, toCost)),
			)
			totalCosts[keys.From] += fromCost
			totalCosts[keys.To] += toCost
		}

		if _, err := fmt.Fprintln(tabWriter, strings.Join(row, "\t")); err != nil {
			return err
		}
	}

	// If there are multiple models, print a Total Costs row.
	if len(r.reports) > 1 {
		row := []string{"Total Cost:"}

		for _, p := range periods {
			keys := p.Keys()
			fromCost, toCost := totalCosts[keys.From], totalCosts[keys.To]
			row = append(row,
				fmt.Sprintf("$%.2f", toCost),
				fmt.Sprintf("$%.2f(%.1f%%)", toCost-fromCost, percentageChange(fromCost, toCost)),
			)
		}

		if _, err := fmt.Fprintln(tabWriter, strings.Join(row, "\t")); err != nil {
			return err
		}
	}

	return tabWriter.Flush()
}

func calculateTotalCostForPeriod(p Period, from Requirements, to Requirements, cm *CostModel) (float64, float64) {
	fromCost := cm.TotalCostForPeriod(p, from)
	toCost := cm.TotalCostForPeriod(p, to)
	return fromCost, toCost
}

func percentageChange(from, to float64) float64 {
	if from == 0 && to == 0 {
		return 0.0
	}
	return ((to - from) / from) * 100.0
}

// displayCostInDollars is a helper to print out the dollars properly if it's negative or positive.
func displayCostInDollars(cost float64) string {
	sign := ""
	if cost < 0 {
		sign, cost = "-", cost*-1
	}

	return fmt.Sprintf("%s$%.2f", sign, cost)
}
