/*
Copyright 2022 Tim Ebert.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/api"
	"github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

const (
	stdin  = "-"
	stdout = "-"
)

var (
	now = time.Now()

	queriesInput io.Reader
	outputDir    string

	prometheusURL = "http://localhost:9091"
	queryRange    = v1.Range{
		Start: now.Add(-15 * time.Minute),
		End:   now,
		Step:  15 * time.Second,
	}
	rateInterval = 5 * time.Minute
)

func main() {
	cmd := &cobra.Command{
		Use:  "measure QUERIES_FILE|-",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			queriesFileName := args[0]
			if queriesFileName == stdin {
				queriesInput = os.Stdin
			} else {
				file, err := os.Open(queriesFileName)
				if err != nil {
					return err
				}
				defer file.Close()
				queriesInput = file
			}

			c, err := newClient()
			if err != nil {
				return fmt.Errorf("error creating prometheus client: %w", err)
			}

			cmd.SilenceUsage = true

			return run(cmd.Context(), c)
		},
	}

	cmd.Flags().StringVarP(&outputDir, "output-dir", "o", outputDir, "Directory to write output files to, - for stdout (defaults to working directory)")
	cmd.Flags().StringVar(&prometheusURL, "prometheus-url", prometheusURL, "URL for querying prometheus")
	cmd.Flags().DurationVar(&rateInterval, "rate-interval", rateInterval, "Interval to use for rate queries")
	cmd.Flags().DurationVar(&queryRange.Step, "step", queryRange.Step, "Query resolution step width")
	cmd.Flags().Var((*timeValue)(&queryRange.Start), "start", "Query start timestamp (RFC33339/duration relative to now/unix timestamp in seconds), inclusive (defaults to now-15m)")
	cmd.Flags().Var((*timeValue)(&queryRange.End), "end", "Query end timestamp (RFC33339/duration relative to now/unix timestamp in seconds), inclusive (defaults to now)")

	if err := cmd.ExecuteContext(signals.SetupSignalHandler()); err != nil {
		fmt.Printf("Error retrieving measurements: %v\n", err)
		os.Exit(1)
	}
}

// timeValue implements pflag.Value for specifying a timestamp in one of the following formats:
//   - RFC33339, e.g. 2006-01-02T15:04:05Z07:00
//   - duration relative to now, e.g. -5m
//   - unix timestamp in seconds, e.g. 1665825136
type timeValue time.Time

func (t *timeValue) Type() string   { return "time" }
func (t *timeValue) String() string { return (*time.Time)(t).UTC().Format(time.RFC3339) }

func (t *timeValue) Set(s string) error {
	// first try RFC3339 timestamp
	tt, err := time.Parse(time.RFC3339, s)
	if err == nil {
		*t = timeValue(tt)
		return nil
	}

	// then try duration
	dd, err := time.ParseDuration(s)
	if err == nil {
		*t = timeValue(time.Now().Add(dd))
		return nil
	}

	// lastly try unix timestamp
	ii, err := strconv.ParseInt(s, 10, 64)
	if err == nil {
		*t = timeValue(time.Unix(ii, 0))
		return nil
	}

	return fmt.Errorf("invalid time value: %q", s)
}

func newClient() (v1.API, error) {
	apiClient, err := api.NewClient(api.Config{
		Address: prometheusURL,
	})
	if err != nil {
		return nil, err
	}

	return v1.NewAPI(apiClient), nil
}

type QueriesConfig struct {
	Queries []Query `yaml:"queries"`
}

type Query struct {
	Name  string `yaml:"name"`
	Query string `yaml:"query"`
}

func run(ctx context.Context, c v1.API) error {
	config, err := decodeQueriesConfig()
	if err != nil {
		return fmt.Errorf("error decoding queries file: %w", err)
	}

	if err = prepareOutputDir(); err != nil {
		return err
	}

	fmt.Printf("Using time range for query: %s\n", rangeToString(queryRange))

	for _, q := range config.Queries {
		fileName := q.Name + ".csv"

		value, err := fetchData(ctx, c, q.Query)
		if err != nil {
			return fmt.Errorf("error fetching data for file %q: %w", fileName, err)
		}

		if err = func() error {
			var out io.Writer
			if outputDir == stdout {
				out = os.Stdout
				fmt.Println("# " + fileName)
			} else {
				file, err := os.OpenFile(filepath.Join(outputDir, fileName), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
				if err != nil {
					return err
				}
				defer file.Close()
				out = file
			}

			return writeResult(value, out)
		}(); err != nil {
			return fmt.Errorf("error writing result %q: %w", fileName, err)
		}
	}

	return nil
}

func decodeQueriesConfig() (*QueriesConfig, error) {
	configData, err := io.ReadAll(queriesInput)
	if err != nil {
		return nil, err
	}

	c := &QueriesConfig{}
	return c, yaml.Unmarshal(configData, c)
}

func prepareOutputDir() error {
	if outputDir == stdout {
		return nil
	}
	if outputDir == "" {
		outputDir = "."
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}

	fmt.Printf("Writing output files to %s\n", outputDir)
	return nil
}

func fetchData(ctx context.Context, c v1.API, query string) (model.Value, error) {
	result, warnings, err := c.QueryRange(ctx, query, queryRange, v1.WithTimeout(10*time.Second))
	if err != nil {
		return nil, err
	}
	if len(warnings) > 0 {
		fmt.Printf("Warnings: %v\n", warnings)
	}

	return result, nil
}

func writeResult(v model.Value, out io.Writer) error {
	matrix, ok := v.(model.Matrix)
	if !ok {
		return fmt.Errorf("unsupported value type %q, expected matrix", v.Type().String())
	}

	if matrix.Len() == 0 {
		return fmt.Errorf("matrix is empty")
	}

	// go maps are unsorted -> need to sort labels by name so that all values end up in the right column
	labelNames := make([]string, 0, len(matrix[0].Metric)-1)
	for name := range matrix[0].Metric {
		if name == model.MetricNameLabel {
			continue
		}
		labelNames = append(labelNames, string(name))
	}
	sort.Strings(labelNames)

	// write header
	w := csv.NewWriter(out)
	if err := w.Write(append([]string{"ts", "value"}, labelNames...)); err != nil {
		return err
	}

	// write contents
	for _, stream := range matrix {
		labelValues := make([]string, len(labelNames))
		for i, name := range labelNames {
			labelValues[i] = string(stream.Metric[model.LabelName(name)])
		}

		for _, samplePair := range stream.Values {
			if err := w.Write(append([]string{
				strconv.FormatInt(samplePair.Timestamp.Unix(), 10),
				samplePair.Value.String(),
			}, labelValues...)); err != nil {
				return err
			}
		}
	}

	w.Flush()
	return w.Error()
}

func rangeToString(r v1.Range) string {
	return fmt.Sprintf("start:%s end:%s step:%s", r.Start.UTC().Format(time.RFC3339), r.End.UTC().Format(time.RFC3339), r.Step.String())
}
