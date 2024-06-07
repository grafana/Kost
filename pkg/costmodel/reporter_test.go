package costmodel

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestReporter_writeSummary(t *testing.T) {
	baseCostModel := &CostModel{
		Cluster: &Cluster{
			Name: "test",
		},
		CPU: Cost{
			Dollars: 1,
			Spot:    1,
			NonSpot: 1,
		},
		RAM: Cost{
			Dollars: 1,
			Spot:    1,
			NonSpot: 1,
		},
		PersistentVolume: Cost{
			Dollars: 1,
			Spot:    1,
			NonSpot: 1,
		},
	}

	fromRequirements := Requirements{
		CPU:              1000,
		Memory:           1024 * 1024 * 1024,
		PersistentVolume: 1024 * 1024 * 1024,
	}

	toRequirements := Requirements{
		CPU:              2000,
		Memory:           1024 * 1024 * 1024 * 2,
		PersistentVolume: 1024 * 1024 * 1024 * 2,
	}

	t.Run("Test a summary with an empty report", func(t *testing.T) {
		var b bytes.Buffer
		want := "PR changed the overall cost by $0.00(0.0%).\nTotal Monthly Cost went from $0.00 to $0.00.\n"
		r := New(&b, "summary")
		r.AddReport(nil, Requirements{}, Requirements{})
		if err := r.Write(); err != nil {
			t.Errorf("writeSummary() must not return an error if reports exist")
		}
		got := b.String()
		if got != want {
			t.Errorf("writeSummary()\n%v\n%v", got, want)
		}
	})

	t.Run("Test a summary with a report", func(t *testing.T) {
		var b bytes.Buffer
		want := "PR changed the overall cost by $2160.00(100.0%).\nTotal Monthly Cost went from $2160.00 to $4320.00.\n"
		r := New(&b, "summary")
		r.AddReport(baseCostModel, fromRequirements, toRequirements)
		if err := r.Write(); err != nil {
			t.Errorf("writeSummary() must not return an error if reports exist")
		}
		got := b.String()
		if got != want {
			t.Errorf("writeSummary()\n%v\n%v", got, want)
		}
	})

	t.Run("Test a summary with decreased report", func(t *testing.T) {
		var b bytes.Buffer
		want := "PR changed the overall cost by -$2160.00(-50.0%).\nTotal Monthly Cost went from $4320.00 to $2160.00.\n"
		r := New(&b, "summary")
		r.AddReport(baseCostModel, toRequirements, fromRequirements)
		if err := r.Write(); err != nil {
			t.Errorf("writeSummary() must not return an error if reports exist")
		}
		got := b.String()
		if got != want {
			t.Errorf("writeSummary()\n%v\n%v", got, want)
		}
	})

	t.Run("Test a table with a single increasing report", func(t *testing.T) {
		var b bytes.Buffer
		r := New(&b, "table")
		want := "Cluster  Total Weekly Cost  Δ Weekly Cost    Total Monthly Cost  Δ Monthly Cost\ntest     $1008.00           $504.00(100.0%)  $4320.00            $2160.00(100.0%)\n"
		r.AddReport(baseCostModel, fromRequirements, toRequirements)
		if err := r.Write(); err != nil {
			t.Errorf("writeSummary() must not return an error if reports exist")
		}
		got := b.String()
		if !strings.Contains(got, want) {
			t.Errorf("Write()\n%v\n%v", got, want)
		}
	})

	t.Run("Test a table with multiple increasing report", func(t *testing.T) {
		var b bytes.Buffer
		r := New(&b, "table")

		r.AddReport(baseCostModel, fromRequirements, toRequirements)
		r.AddReport(baseCostModel, fromRequirements, toRequirements)
		if err := r.Write(); err != nil {
			t.Errorf("writeSummary() must not return an error if reports exist")
		}
		got := b.String()

		if !strings.Contains(got, "Total Cost:") {
			t.Errorf("Write() with multiple rows did not include the total cost row.")
		}
	})

	t.Run("Test a table with a single decreasing report", func(t *testing.T) {
		var b bytes.Buffer
		r := New(&b, "table")
		// @Pokom: This kind of sucks to test this way, but I couldn't think of a better way.
		// I don't think we need to test here that it prints out correctly. Ideally I want to check to see that the
		// Printed numbers are accurate.
		want := "Cluster  Total Weekly Cost  Δ Weekly Cost     Total Monthly Cost  Δ Monthly Cost\ntest     $504.00            -$504.00(-50.0%)  $2160.00            -$2160.00(-50.0%)\n"
		r.AddReport(baseCostModel, toRequirements, fromRequirements)
		if err := r.Write(); err != nil {
			t.Errorf("writeSummary() must not return an error if reports exist")
		}
		got := b.String()
		if !strings.Contains(got, want) {
			t.Errorf("Write()\n%v\n%v", got, want)
		}
	})
}

func Test_percentageChange(t *testing.T) {
	tests := map[string]struct {
		from float64
		to   float64
		want float64
	}{
		"Test a percentage change with 0 for from and to": {
			from: 0,
			to:   0,
			want: 0,
		},
		"Test a percentage change with a negative value": {
			from: 100,
			to:   50,
			want: -50,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := percentageChange(tt.from, tt.to); got != tt.want {
				t.Errorf("percentageChange() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_calculateTotalCostForPeriod(t *testing.T) {
	cost := Cost{
		Spot:    1,
		NonSpot: 1,
		Dollars: 1,
	}
	cm := &CostModel{
		Cluster: &Cluster{
			Name: "test",
		},
		CPU:              cost,
		RAM:              cost,
		PersistentVolume: cost,
	}
	var p Period = Monthly

	tests := map[string]struct {
		cm               *CostModel
		from             Requirements
		to               Requirements
		changeMultiplier float64
	}{
		"no change should result in no cost": {
			cm: cm,
			from: Requirements{
				CPU:              1,
				Memory:           1,
				PersistentVolume: 1,
			},
			to: Requirements{
				CPU:              1,
				Memory:           1,
				PersistentVolume: 1,
			},
			changeMultiplier: 1,
		},
		"double in resources should result in a double in cost": {
			cm: cm,
			from: Requirements{
				CPU:              1000,
				Memory:           1024 * 1024 * 1024,
				PersistentVolume: 1000,
			},

			to: Requirements{
				CPU:              2000,
				Memory:           1024 * 1024 * 1024 * 2,
				PersistentVolume: 2000,
			},
			changeMultiplier: 2,
		},
		"halving in resources should result in a halving in cost": {
			cm: cm,
			from: Requirements{
				CPU:              1000,
				Memory:           1024 * 1024 * 1024,
				PersistentVolume: 1000,
			},
			to: Requirements{
				CPU:              500,
				Memory:           1024 * 1024 * 1024 / 2,
				PersistentVolume: 500,
			},
			changeMultiplier: 0.5,
		},
	}
	for name, test := range tests {
		from, to := calculateTotalCostForPeriod(p, test.from, test.to, test.cm)
		if from*test.changeMultiplier != to {
			t.Errorf("%s: from %v != to %v * changeMultiplier %v", name, from, to, test.changeMultiplier)
		}
	}
}

func TestReporter_Write(t *testing.T) {
	t.Run("no reports", func(t *testing.T) {
		for _, rt := range []ReportType{Table, Summary, Markdown} {
			rt := string(rt)
			t.Run(rt, func(t *testing.T) {
				var s strings.Builder
				err := New(&s, rt).Write()
				if !errors.Is(err, ErrNoReports) {
					t.Fatalf("expecting ErrNoReports, got %v", err)
				}
				if s.Len() > 0 {
					t.Fatalf("expecting reporter not to write anything, got %q", s.String())
				}
			})
		}
	})
}
