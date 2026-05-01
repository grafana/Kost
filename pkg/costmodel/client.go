package costmodel

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"strings"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	configutil "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
)

const (
	queryCostPerCpu = `
	avg by (price_tier) (
		cloudcost_aws_ec2_instance_cpu_usd_per_core_hour{cluster_name="%s"}
		or
		cloudcost_azure_aks_instance_cpu_usd_per_core_hour{cluster_name="%s"}
		or
		cloudcost_gcp_gke_instance_cpu_usd_per_core_hour{cluster_name="%s"}
)
`
	queryMemoryCost = `
	avg by (price_tier) (
		cloudcost_aws_ec2_instance_memory_usd_per_gib_hour{cluster_name="%s"}
		or
		cloudcost_azure_aks_instance_memory_usd_per_gib_hour{cluster_name="%s"}
		or
		cloudcost_gcp_gke_instance_memory_usd_per_gib_hour{cluster_name="%s"}
)
`

	// TODO(@Pokom): update this query with azure's PVC's cost once https://github.com/grafana/cloudcost-exporter/issues/236 is merged in
	queryPersistentVolumeCost = `
			avg(
				cloudcost_aws_ec2_persistent_volume_usd_per_hour{persistentvolume!="", state="in-use"}
				/ on (persistentvolume) group_left() (
                    kube_persistentvolume_capacity_bytes{cluster="%s"} / 1e9
                )
            )
			or
				avg(
					cloudcost_gcp_gke_persistent_volume_usd_per_hour{persistentvolume!="", use_status="in-use", cluster_name="%s"}
					/ on (persistentvolume) group_left() (
						kube_persistentvolume_capacity_bytes{cluster="%s"} / 1e9
				)
			)
			or
			avg(
				pv_hourly_cost{cluster="%s"}
			)
`

	queryAverageNodeCount = `
		avg_over_time(
			sum(nodepool:node:sum{cluster="%s"})[30d:1d]
		)
	`

	// queryHPATargeting filters kube_horizontalpodautoscaler_info by the workload it targets.
	// The horizontalpodautoscaler label on a hit holds the HPA name (also used by KEDA-managed HPAs).
	queryHPATargeting = `kube_horizontalpodautoscaler_info{cluster="%s", namespace="%s", scaletargetref_kind="%s", scaletargetref_name="%s"}`

	// queryObservedReplicas reports the average actual replica count over a 7d window.
	// Format args: metric, cluster, namespace, kindLabel, name.
	queryObservedReplicas = `avg(avg_over_time(%s{cluster="%s", namespace="%s", %s="%s"}[7d]))`
)

// ErrNoResults is the error returned when querying for costs returns
// no results.
var (
	ErrNoResults          = errors.New("no cost results")
	ErrBadQuery           = errors.New("bad query")
	ErrNilConfig          = errors.New("client config is nil")
	ErrEmptyAddress       = errors.New("client address can't be empty")
	ErrProdConfigMissing  = errors.New("prod config is missing")
	ErrHPADetectionFailed = errors.New("hpa detection failed")
	ErrUnsupportedKind    = errors.New("unsupported workload kind")
)

// Client is a client for the cost model.
type Client struct {
	client api.Client
}

// Clients bundles the dev and prod client in one struct.
type Clients struct {
	Prod *Client
	Dev  *Client
}

// ClientConfig is the configuration for the cost model client.
type ClientConfig struct {
	Address        string
	HTTPConfigFile string
	Username       string
	Password       string
}

// NewClient creates a new cost model client with the given configuration.
func NewClient(config *ClientConfig) (*Client, error) {
	cfg := &configutil.HTTPClientConfig{}
	if config == nil {
		return nil, ErrNilConfig
	}
	if config.Address == "" {
		return nil, ErrEmptyAddress
	}
	if config.HTTPConfigFile != "" {
		fmt.Printf("loading http config file: %s\n", config.HTTPConfigFile)
		var err error
		cfg, _, err = configutil.LoadHTTPConfigFile(config.HTTPConfigFile)
		if err != nil {
			return nil, fmt.Errorf("error loading http config file: %v", err)
		}
	} else if config.Username != "" && config.Password != "" {
		fmt.Println("using basic auth")
		cfg = &configutil.HTTPClientConfig{
			BasicAuth: &configutil.BasicAuth{
				Username: config.Username,
				Password: configutil.Secret(config.Password),
			},
		}
	} else {
		fmt.Println("HTTP config file and basic auth not provided, using no authentication")
	}

	roundTripper, err := configutil.NewRoundTripperFromConfig(*cfg, "grafana-kost-estimator", configutil.WithHTTP2Disabled(), configutil.WithUserAgent("grafana-kost-estimator"))
	if err != nil {
		return nil, fmt.Errorf("error creating round tripper: %v", err)
	}
	client, err := api.NewClient(api.Config{Address: config.Address, RoundTripper: roundTripper})
	if err != nil {
		return nil, err
	}
	return &Client{
		client: client,
	}, nil
}

// NewClients creates a new cost model clients with the given configuration.
func NewClients(prodConfig, devConfig *ClientConfig) (*Clients, error) {
	var clients Clients
	prometheusProdClient, err := NewClient(prodConfig)
	if err != nil {
		return nil, ErrProdConfigMissing
	}
	clients.Prod = prometheusProdClient
	// It isn't necessary to initiate the dev client therefore we ignore potential errors from this
	prometheusDevClient, _ := NewClient(devConfig)
	clients.Dev = prometheusDevClient
	return &clients, nil
}

// GetCostPerCPU returns the average cost per CPU for a given cluster.
func (c *Client) GetCostPerCPU(ctx context.Context, cluster string) (Cost, error) {
	query := fmt.Sprintf(queryCostPerCpu, cluster, cluster, cluster)
	results, err := c.query(ctx, query)
	if err != nil {
		return Cost{}, err
	}
	return c.parseResults(results)
}

// GetMemoryCost returns the cost per memory for a given cluster
func (c *Client) GetMemoryCost(ctx context.Context, cluster string) (Cost, error) {
	query := fmt.Sprintf(queryMemoryCost, cluster, cluster, cluster)
	results, err := c.query(ctx, query)
	if err != nil {
		return Cost{}, err
	}
	return c.parseResults(results)
}

// GetNodeCount returns the average number of nodes over 30 days for a given cluster
func (c *Client) GetNodeCount(ctx context.Context, cluster string) (int, error) {
	query := fmt.Sprintf(queryAverageNodeCount, cluster)
	results, err := c.query(ctx, query)
	if err != nil {
		return 0, ErrBadQuery
	}

	result := results.(model.Vector)
	if len(result) == 0 {
		return 0, ErrNoResults
	}

	return int(result[0].Value), nil
}

// GetObservedReplicas returns the 7-day average replica count for the given workload
// from kube-state-metrics, regardless of who scales it (HPA, KEDA, manual). Used to
// substitute manifest replicas with reality for HPA-managed workloads.
// Returns ErrUnsupportedKind for kinds without a replica-count metric (DaemonSet, Pod, Job).
// Returns ErrNoResults if the query succeeds but has no samples (e.g., brand-new workload
// with no history) — callers should surface this rather than silently fall back.
func (c *Client) GetObservedReplicas(ctx context.Context, cluster, namespace, kind, name string) (float64, error) {
	metric, kindLabel, err := replicaMetricForKind(kind)
	if err != nil {
		return 0, err
	}
	query := fmt.Sprintf(queryObservedReplicas, metric, cluster, namespace, kindLabel, name)
	results, err := c.query(ctx, query)
	if err != nil {
		return 0, ErrBadQuery
	}
	vec, ok := results.(model.Vector)
	if !ok {
		return 0, ErrBadQuery
	}
	if len(vec) == 0 {
		return 0, ErrNoResults
	}
	return float64(vec[0].Value), nil
}

// replicaMetricForKind returns the kube-state-metrics metric name and the label
// holding the workload name for a given Kubernetes kind.
func replicaMetricForKind(kind string) (metric, label string, err error) {
	switch kind {
	case "Deployment":
		return "kube_deployment_status_replicas", "deployment", nil
	case "StatefulSet":
		return "kube_statefulset_replicas", "statefulset", nil
	default:
		return "", "", fmt.Errorf("%w: %s", ErrUnsupportedKind, kind)
	}
}

// HPATargeting returns the name of the HorizontalPodAutoscaler targeting the given
// (cluster, namespace, kind, name) workload, or an empty string if no HPA targets it.
// Works for vanilla HPAs and KEDA-managed HPAs (KEDA creates a regular HPA underneath).
// Transport or unexpected response shape errors are wrapped with ErrHPADetectionFailed
// so callers can distinguish "definitely not HPA-managed" from "we couldn't tell."
func (c *Client) HPATargeting(ctx context.Context, cluster, namespace, kind, name string) (string, error) {
	query := fmt.Sprintf(queryHPATargeting, cluster, namespace, kind, name)
	results, err := c.query(ctx, query)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrHPADetectionFailed, err)
	}
	vec, ok := results.(model.Vector)
	if !ok {
		return "", fmt.Errorf("%w: unexpected result type %T", ErrHPADetectionFailed, results)
	}
	if len(vec) == 0 {
		return "", nil
	}
	hpa := string(vec[0].Metric["horizontalpodautoscaler"])
	if hpa == "" {
		return "", fmt.Errorf("%w: missing horizontalpodautoscaler label", ErrHPADetectionFailed)
	}
	return hpa, nil
}

// GetCostForPersistentVolume returns the average cost per persistent volume for a given cluster
func (c *Client) GetCostForPersistentVolume(ctx context.Context, cluster string) (Cost, error) {
	query := fmt.Sprintf(queryPersistentVolumeCost, cluster, cluster, cluster, cluster)
	results, err := c.query(ctx, query)
	if err != nil {
		return Cost{}, err
	}
	return c.parseResults(results)
}

func (c *Client) parseResults(results model.Value) (Cost, error) {
	result := results.(model.Vector)

	if len(result) == 0 {
		return Cost{}, ErrNoResults
	}

	var cost Cost
	for _, sample := range result {
		value := float64(sample.Value)

		switch sample.Metric["spot"] {
		case "true":
			cost.Spot = value
		case "false":
			cost.NonSpot = value
		default:
			// This is when there is no spot/non-spot label
			cost.Dollars = value
		}
		// Handles the case for cloudcost exporter metrics where `price_tier` is the label for spot/non-spot
		// TODO: Delete after removing support for OpenCost
		switch sample.Metric["price_tier"] {
		case "ondemand":
			cost.NonSpot = value
		case "spot":
			cost.Spot = value
		default:
			// This is when there is no spot/non-spot label
			cost.Dollars = value
		}
	}

	return cost, nil
}

// query queries prometheus with the given query
func (c *Client) query(ctx context.Context, query string) (model.Value, error) {
	api := v1.NewAPI(c.client)
	results, warnings, err := api.Query(ctx, query, time.Now())
	if err != nil {
		return nil, err
	}

	if len(warnings) > 0 {
		// TODO this isn't probably something we want to. Let's
		// revisit the feasibility of receiving warnings later.
		log.Printf("Warnings: %v", warnings)
	}
	return results, nil
}

// GetClusterCosts returns the cost for a cluster and differentiate for dev and prod clusters
func (c *Clients) GetClusterCosts(ctx context.Context, cluster string) (*CostModel, error) {
	start := time.Now()
	defer func() {
		slog.Info("GetClusterCosts", "cluster", cluster, "duration", time.Since(start))
	}()
	var cost *CostModel
	var err error
	// if dev client is present
	client := c.Prod
	if c.Dev != nil && strings.HasPrefix(cluster, "dev-") {
		client = c.Dev
	}
	cost, err = GetCostModelForCluster(ctx, client, cluster)
	if err != nil {
		// TODO here we should probably return an error like below
		return nil, fmt.Errorf("fetching cost model for cluster %s: %w", cluster, err)
	}
	return cost, nil
}
