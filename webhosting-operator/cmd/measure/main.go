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
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/api"
	"github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/util/sets"
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
	outputPrefix string

	prometheusURL = "http://localhost:9091"
	queryRange    = v1.Range{
		Start: now.Add(-15 * time.Minute),
		End:   now,
		Step:  15 * time.Second,
	}
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
	cmd.Flags().StringVar(&outputPrefix, "output-prefix", outputPrefix, "Prefix to prepend to all output files")
	cmd.Flags().StringVar(&prometheusURL, "prometheus-url", prometheusURL, "URL for querying prometheus")
	cmd.Flags().DurationVar(&queryRange.Step, "step", queryRange.Step, "Query resolution step width")
	cmd.Flags().Var((*timeValue)(&queryRange.Start), "start", "Query start timestamp (RFC3339/duration relative to now/unix timestamp in seconds), inclusive (defaults to now-15m)")
	cmd.Flags().Var((*timeValue)(&queryRange.End), "end", "Query end timestamp (RFC3339/duration relative to now/unix timestamp in seconds), inclusive (defaults to now)")

	if err := cmd.ExecuteContext(signals.SetupSignalHandler()); err != nil {
		os.Exit(1)
	}
}

// timeValue implements pflag.Value for specifying a timestamp in one of the following formats:
//   - RFC3339, e.g. 2006-01-02T15:04:05Z07:00
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

const (
	QueryTypeInstant = "instant"
	QueryTypeRange   = "range"
)

type QueriesConfig struct {
	Queries []Query `yaml:"queries"`
}

type Query struct {
	Name string `yaml:"name"`
	// range or instant, defaults to range.
	// If instant is specified, the $__range variable in the query is substituted by the configured range's duration.
	Type     string `yaml:"type,omitempty"`
	Query    string `yaml:"query"`
	Optional bool   `yaml:"optional,omitempty"`
	// upper threshold for values
	SLO *float64 `yaml:"slo,omitempty"`
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

	var (
		slosChecked = false
		slosMet     = true
	)

	for _, q := range config.Queries {
		data, err := q.fetchData(ctx, c)
		if err != nil {
			return fmt.Errorf("error fetching data for query %q: %w", q.Name, err)
		}

		if data.IsEmpty() {
			if q.Optional {
				fmt.Printf("Skipping output for query %q as data is empty\n", q.Name)
				continue
			}
			return fmt.Errorf("data is empty for query %q, but query is required", q.Name)
		}

		if err = q.writeResult(data); err != nil {
			return fmt.Errorf("error writing result: %w", err)
		}

		if checked, ok := q.verifySLO(data); checked {
			slosChecked = true
			if !ok {
				slosMet = false
			}
		}
	}

	if slosChecked {
		if !slosMet {
			return fmt.Errorf("❌ SLO verifications failed")
		}

		fmt.Println("✅ SLO verifications succeeded")
	} else {
		fmt.Println("ℹ️ No SLOs defined")
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

func (q Query) fetchData(ctx context.Context, c v1.API) (metricData, error) {
	opts := []v1.Option{v1.WithTimeout(10 * time.Second)}

	var (
		data     metricData
		result   model.Value
		warnings v1.Warnings
		err      error
	)

	switch q.Type {
	case QueryTypeRange, "":
		result, warnings, err = c.QueryRange(ctx, q.Query, queryRange, opts...)
		if err != nil {
			return nil, err
		}

		matrix, ok := result.(model.Matrix)
		if !ok {
			return nil, fmt.Errorf("unexpected value type %q for query %q, expected matrix", result.Type().String(), q.Name)
		}
		data = &matrixData{Matrix: matrix}

	case QueryTypeInstant:
		rangeString := queryRange.End.Sub(queryRange.Start).String()
		query := q.Query
		query = strings.ReplaceAll(query, "$__range", rangeString)

		result, warnings, err = c.Query(ctx, query, queryRange.End, opts...)
		if err != nil {
			return nil, err
		}

		vector, ok := result.(model.Vector)
		if !ok {
			return nil, fmt.Errorf("unexpected value type %q for query %q, expected vector", result.Type().String(), q.Name)
		}
		data = &vectorData{Vector: vector}

	default:
		return nil, fmt.Errorf("unsupported query type %q", q.Type)
	}

	if len(warnings) > 0 {
		fmt.Printf("Warnings: %v\n", warnings)
	}

	return data, nil
}

func (q Query) writeResult(data metricData) error {
	fileName := outputPrefix + q.Name + ".csv"

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

	// go maps are unsorted -> need to sort labels by name so that all values end up in the right column
	labelNames := data.GetLabelNames()
	slices.Sort(labelNames)

	// write header
	w := csv.NewWriter(out)
	if err := w.Write(append([]string{"ts", "value"}, toStringSlice(labelNames)...)); err != nil {
		return err
	}

	// write contents
	data.Reset()
	for {
		value := data.NextValue()
		if value.IsZero() { // end of results
			break
		}

		labelValues := make([]string, len(labelNames))
		for i, name := range labelNames {
			labelValues[i] = string(value.metric[name])
		}

		if err := w.Write(append([]string{
			strconv.FormatInt(value.time.Unix(), 10),
			strconv.FormatFloat(value.value, 'f', -1, 64),
		}, labelValues...)); err != nil {
			return err
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return err
	}

	fmt.Printf("Successfully written output to %s\n", fileName)
	return nil
}

func (q Query) verifySLO(data metricData) (checked bool, ok bool) {
	if q.SLO == nil {
		return false, true
	}

	var allFailures []string

	data.Reset()
	for {
		value := data.NextValue()
		if value.IsZero() { // end of results
			break
		}

		if math.IsNaN(value.value) || value.value <= *q.SLO {
			continue
		}

		allFailures = append(allFailures, fmt.Sprintf("%s => %f @[%s]", value.metric, value.value, value.time.Format(time.RFC3339)))
	}

	if len(allFailures) > 0 {
		indent := "- "
		fmt.Printf("❌ SLO for query %q (<= %f) is not met:\n%s\n", q.Name, *q.SLO, indent+strings.Join(allFailures, "\n"+indent))
		return true, false
	}

	fmt.Printf("✅ SLO for query %q (<= %f) met\n", q.Name, *q.SLO)
	return true, true
}

func rangeToString(r v1.Range) string {
	return fmt.Sprintf("start:%s end:%s step:%s", r.Start.UTC().Format(time.RFC3339), r.End.UTC().Format(time.RFC3339), r.Step.String())
}

// metricData gives cursor-style access to metric values, which could either be a matrix (for range queries) or a vector
// (for instant queries).
type metricData interface {
	// IsEmpty returns true if the query result was empty.
	IsEmpty() bool
	// GetLabelNames returns an unsorted list of label names of the first metric.
	GetLabelNames() model.LabelNames
	// Reset resets the cursor.
	Reset()
	// NextValue moves the cursor to the next metric value and returns it. It returns a zero metricValue at the end.
	NextValue() metricValue
}

type metricValue struct {
	metric model.Metric
	time   time.Time
	value  float64
}

func (m metricValue) IsZero() bool {
	return m.time.IsZero()
}

type matrixData struct {
	model.Matrix

	cursor struct {
		metric, value int
	}
}

func (m *matrixData) IsEmpty() bool {
	return m.Matrix.Len() == 0
}

func (m *matrixData) GetLabelNames() model.LabelNames {
	labels := sets.New[model.LabelName]()

	for _, metric := range m.Matrix {
		for labelName := range metric.Metric {
			if labelName == model.MetricNameLabel {
				continue
			}
			labels.Insert(labelName)
		}
	}

	return labels.UnsortedList()
}

func (m *matrixData) Reset() {
	m.cursor.metric = 0
	m.cursor.value = 0
}

func (m *matrixData) NextValue() metricValue {
	if m.IsEmpty() {
		return metricValue{}
	}

	// move to next metric if we already visited all values in the current metric
	if m.cursor.value >= len(m.Matrix[m.cursor.metric].Values) {
		m.cursor.metric++
		m.cursor.value = 0
	}

	// end of results
	if m.cursor.metric >= m.Len() {
		return metricValue{}
	}

	sampleStream := m.Matrix[m.cursor.metric]
	value := sampleStream.Values[m.cursor.value]

	// move on to the next value in the current metric
	m.cursor.value++

	return metricValue{
		metric: sampleStream.Metric,
		time:   value.Timestamp.Time(),
		value:  float64(value.Value),
	}
}

type vectorData struct {
	model.Vector

	cursor int
}

func (v *vectorData) IsEmpty() bool {
	return v.Vector.Len() == 0
}

func (v *vectorData) GetLabelNames() model.LabelNames {
	labels := sets.New[model.LabelName]()

	for _, metric := range v.Vector {
		for labelName := range metric.Metric {
			if labelName == model.MetricNameLabel {
				continue
			}
			labels.Insert(labelName)
		}
	}

	return labels.UnsortedList()
}

func (v *vectorData) Reset() {
	v.cursor = 0
}

func (v *vectorData) NextValue() metricValue {
	if v.IsEmpty() {
		return metricValue{}
	}

	// end of results
	if v.cursor >= len(v.Vector) {
		return metricValue{}
	}

	sample := v.Vector[v.cursor]

	// move on to the next sample
	v.cursor++

	return metricValue{
		metric: sample.Metric,
		time:   sample.Timestamp.Time(),
		value:  float64(sample.Value),
	}
}

func toStringSlice[S ~[]E, E ~string](s S) []string {
	out := make([]string, 0, len(s))
	for _, v := range s {
		out = append(out, string(v))
	}
	return out
}
