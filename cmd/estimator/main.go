package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/grafana/kost/pkg/costmodel"
)

func main() {
	var fromFile, toFile, prometheusAddress, httpConfigFile, reportType, username, password string
	flag.StringVar(&fromFile, "from", "", "The file to compare from")
	flag.StringVar(&toFile, "to", "", "The file to compare to")
	flag.StringVar(&prometheusAddress, "prometheus.address", "http://localhost:9093/prometheus", "The Address of the prometheus server")
	flag.StringVar(&httpConfigFile, "http.config.file", "", "The path to the http config file")
	flag.StringVar(&username, "username", "", "Mimir username")
	flag.StringVar(&password, "password", "", "Mimir password")
	flag.StringVar(&reportType, "report.type", "table", "The type of report to generate. Options are: table, summary")
	flag.Parse()

	clusters := flag.Args()

	ctx := context.Background()
	if err := run(ctx, fromFile, toFile, prometheusAddress, httpConfigFile, reportType, username, password, clusters); err != nil {
		fmt.Printf("Could not run: %s\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, fromFile, toFile, address, httpConfigFile, reportType, username, password string, clusters []string) error {
	from, err := os.ReadFile(fromFile)
	if err != nil {
		return fmt.Errorf("could not read file: %s", err)
	}

	// TODO: If the to file is not set, we should default to printing out the cost of the current configuration
	to, err := os.ReadFile(toFile)
	if err != nil {
		return fmt.Errorf("could not read file: %s", err)
	}

	client, err := costmodel.NewClient(&costmodel.ClientConfig{
		Address:        address,
		HTTPConfigFile: httpConfigFile,
		Username:       username,
		Password:       password,
	})

	if err != nil {
		return fmt.Errorf("could not create cost model client: %s", err)
	}

	reporter := costmodel.New(os.Stdout, reportType)

	for _, cluster := range clusters {
		cost, err := costmodel.GetCostModelForCluster(ctx, client, cluster)
		if err != nil {
			return fmt.Errorf("could not get costmodel for cluster(%s): %s", cluster, err)
		}

		fromRequests, err := costmodel.ParseManifest(from, cost)
		if err != nil {
			return fmt.Errorf("could not parse manifest file(%s): %s", fromFile, err)
		}

		toRequests, err := costmodel.ParseManifest(to, cost)
		if err != nil {
			return fmt.Errorf("could not parse manifest file(%s): %s", toFile, err)
		}
		reporter.AddReport(cost, fromRequests, toRequests)
	}

	return reporter.Write()
}
