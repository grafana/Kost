package costmodel

import (
	"context"
	"fmt"

	"github.com/grafana/kost/pkg/costmodel/utils"
)

type Period float64

const (
	Hourly  Period = 1
	Daily          = 24
	Weekly         = 24 * 7
	Monthly        = 24 * 30
	Yearly         = 24 * 365
)

func (p Period) String() string {
	switch p {
	case Hourly:
		return "hourly"
	case Daily:
		return "daily"
	case Weekly:
		return "weekly"
	case Monthly:
		return "monthly"
	case Yearly:
		return "yearly"
	default:
		panic("unrecognized value for Period")
	}
}

type PeriodKeys struct {
	To    string
	Delta string
	From  string
}

func (p Period) Keys() PeriodKeys {
	to := fmt.Sprintf("%s-to", p)
	delta := fmt.Sprintf("%s-delta", p)
	from := fmt.Sprintf("%s-from", p)
	return PeriodKeys{To: to, Delta: delta, From: from}
}

// Cost represents the _hourly_ cost of a resource in USD.
// If the cluster does not have pricing data for spot nodes, then Dollars will be set.
type Cost struct {
	Dollars float64
	Spot    float64
	NonSpot float64
}

func (c Cost) SpotCPUForPeriod(p Period, r int64) float64 {
	return float64(r) / 1000 * c.Spot * float64(p)
}

func (c Cost) NonSpotCPUForPeriod(p Period, r int64) float64 {
	return float64(r) / 1000 * c.NonSpot * float64(p)
}

// DollarsForPeriod returns the cost of a resource in USD for a given period.
// Primarily used by PersistentVolumeClaims which do not have spot/non spot pricing.
func (c Cost) DollarsForPeriod(p Period, r int64) float64 {
	return utils.BytesToGiB(r) * c.Dollars * float64(p)
}

func (c Cost) SpotMemoryForPeriod(p Period, r int64) float64 {
	return utils.BytesToGiB(r) * c.Spot * float64(p)
}

func (c Cost) NonSpotMemoryForPeriod(p Period, r int64) float64 {
	return utils.BytesToGiB(r) * c.NonSpot * float64(p)
}

func (c Cost) SpotYearly(cpuReq int64) float64 { return c.SpotCPUForPeriod(Yearly, cpuReq) }

func (c Cost) NonSpotYearly(cpuReq int64) float64 { return c.NonSpotCPUForPeriod(Yearly, cpuReq) }

func (c Cost) DollarsYearly(memReq int64) float64 { return c.DollarsForPeriod(Yearly, memReq) }

// CostModel represents the cost of each resource for a specific cluster
type CostModel struct {
	Cluster          *Cluster
	CPU              Cost
	RAM              Cost
	PersistentVolume Cost
}

type Cluster struct {
	Name      string
	NodeCount int
}

func GetCostModelForCluster(ctx context.Context, client *Client, cluster string) (*CostModel, error) {
	cpu, err := client.GetCostPerCPU(ctx, cluster)
	if err != nil {
		return nil, fmt.Errorf("could not find CPU cost: %s", err)
	}

	memory, err := client.GetMemoryCost(ctx, cluster)
	if err != nil {
		return nil, fmt.Errorf("could not find memory cost: %s", err)
	}

	pvc, err := client.GetCostForPersistentVolume(ctx, cluster)
	if err != nil {
		return nil, fmt.Errorf("could not find persistent volume cost: %s", err)
	}

	nodeCount, err := client.GetNodeCount(ctx, cluster)
	if err != nil {
		return nil, fmt.Errorf("could not find node count: %s", err)
	}

	return &CostModel{
		Cluster:          &Cluster{Name: cluster, NodeCount: nodeCount},
		CPU:              cpu,
		RAM:              memory,
		PersistentVolume: pvc,
	}, nil
}

// TotalCostForPeriod calculates the costs of each resource on the CostModel and returns the sum of the costs
func (c *CostModel) TotalCostForPeriod(p Period, r Requirements) float64 {
	cpuCost := c.CPU.NonSpotCPUForPeriod(p, r.CPU)
	ramCost := c.RAM.NonSpotMemoryForPeriod(p, r.Memory)
	pvCost := c.PersistentVolume.DollarsForPeriod(p, r.PersistentVolume)
	return cpuCost + ramCost + pvCost
}
