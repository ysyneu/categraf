// Copyright 2021 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package collector

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"

	"github.com/prometheus/client_golang/prometheus"
)

type snapshotMetric struct {
	Type   prometheus.ValueType
	Desc   *prometheus.Desc
	Value  func(snapshotStats SnapshotStatDataResponse) float64
	Labels func(repositoryName string, snapshotStats SnapshotStatDataResponse) []string
}

type repositoryMetric struct {
	Type   prometheus.ValueType
	Desc   *prometheus.Desc
	Value  func(snapshotsStats SnapshotStatsResponse) float64
	Labels func(repositoryName string) []string
}

var (
	defaultSnapshotLabels      = []string{"repository", "state", "version"}
	defaultSnapshotLabelValues = func(repositoryName string, snapshotStats SnapshotStatDataResponse) []string {
		return []string{repositoryName, snapshotStats.State, snapshotStats.Version}
	}
	defaultSnapshotRepositoryLabels      = []string{"repository"}
	defaultSnapshotRepositoryLabelValues = func(repositoryName string) []string {
		return []string{repositoryName}
	}
)

// Snapshots information struct
type Snapshots struct {
	client *http.Client
	url    *url.URL

	snapshotMetrics   []*snapshotMetric
	repositoryMetrics []*repositoryMetric
}

// NewSnapshots defines Snapshots Prometheus metrics
func NewSnapshots(client *http.Client, url *url.URL) *Snapshots {
	return &Snapshots{
		client: client,
		url:    url,

		snapshotMetrics: []*snapshotMetric{
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "snapshot_stats", "snapshot_number_of_indices"),
					"Number of indices in the last snapshot",
					defaultSnapshotLabels, nil,
				),
				Value: func(snapshotStats SnapshotStatDataResponse) float64 {
					return float64(len(snapshotStats.Indices))
				},
				Labels: defaultSnapshotLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "snapshot_stats", "snapshot_start_time_timestamp"),
					"Last snapshot start timestamp",
					defaultSnapshotLabels, nil,
				),
				Value: func(snapshotStats SnapshotStatDataResponse) float64 {
					return float64(snapshotStats.StartTimeInMillis / 1000)
				},
				Labels: defaultSnapshotLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "snapshot_stats", "snapshot_end_time_timestamp"),
					"Last snapshot end timestamp",
					defaultSnapshotLabels, nil,
				),
				Value: func(snapshotStats SnapshotStatDataResponse) float64 {
					return float64(snapshotStats.EndTimeInMillis / 1000)
				},
				Labels: defaultSnapshotLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "snapshot_stats", "snapshot_number_of_failures"),
					"Last snapshot number of failures",
					defaultSnapshotLabels, nil,
				),
				Value: func(snapshotStats SnapshotStatDataResponse) float64 {
					return float64(len(snapshotStats.Failures))
				},
				Labels: defaultSnapshotLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "snapshot_stats", "snapshot_total_shards"),
					"Last snapshot total shards",
					defaultSnapshotLabels, nil,
				),
				Value: func(snapshotStats SnapshotStatDataResponse) float64 {
					return float64(snapshotStats.Shards.Total)
				},
				Labels: defaultSnapshotLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "snapshot_stats", "snapshot_failed_shards"),
					"Last snapshot failed shards",
					defaultSnapshotLabels, nil,
				),
				Value: func(snapshotStats SnapshotStatDataResponse) float64 {
					return float64(snapshotStats.Shards.Failed)
				},
				Labels: defaultSnapshotLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "snapshot_stats", "snapshot_successful_shards"),
					"Last snapshot successful shards",
					defaultSnapshotLabels, nil,
				),
				Value: func(snapshotStats SnapshotStatDataResponse) float64 {
					return float64(snapshotStats.Shards.Successful)
				},
				Labels: defaultSnapshotLabelValues,
			},
		},
		repositoryMetrics: []*repositoryMetric{
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "snapshot_stats", "number_of_snapshots"),
					"Number of snapshots in a repository",
					defaultSnapshotRepositoryLabels, nil,
				),
				Value: func(snapshotsStats SnapshotStatsResponse) float64 {
					return float64(len(snapshotsStats.Snapshots))
				},
				Labels: defaultSnapshotRepositoryLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "snapshot_stats", "oldest_snapshot_timestamp"),
					"Timestamp of the oldest snapshot",
					defaultSnapshotRepositoryLabels, nil,
				),
				Value: func(snapshotsStats SnapshotStatsResponse) float64 {
					if len(snapshotsStats.Snapshots) == 0 {
						return 0
					}
					return float64(snapshotsStats.Snapshots[0].StartTimeInMillis / 1000)
				},
				Labels: defaultSnapshotRepositoryLabelValues,
			},
			{
				Type: prometheus.GaugeValue,
				Desc: prometheus.NewDesc(
					prometheus.BuildFQName(namespace, "snapshot_stats", "latest_snapshot_timestamp_seconds"),
					"Timestamp of the latest SUCCESS or PARTIAL snapshot",
					defaultSnapshotRepositoryLabels, nil,
				),
				Value: func(snapshotsStats SnapshotStatsResponse) float64 {
					for i := len(snapshotsStats.Snapshots) - 1; i >= 0; i-- {
						var snap = snapshotsStats.Snapshots[i]
						if snap.State == "SUCCESS" || snap.State == "PARTIAL" {
							return float64(snap.StartTimeInMillis / 1000)
						}
					}
					return 0
				},
				Labels: defaultSnapshotRepositoryLabelValues,
			},
		},
	}
}

// Describe add Snapshots metrics descriptions
func (s *Snapshots) Describe(ch chan<- *prometheus.Desc) {
	for _, metric := range s.snapshotMetrics {
		ch <- metric.Desc
	}
	for _, metric := range s.repositoryMetrics {
		ch <- metric.Desc
	}

}

func (s *Snapshots) getAndParseURL(u *url.URL, data interface{}) error {
	res, err := s.client.Get(u.String())
	if err != nil {
		return fmt.Errorf("failed to get from %s://%s:%s%s: %s",
			u.Scheme, u.Hostname(), u.Port(), u.Path, err)
	}

	defer func() {
		err = res.Body.Close()
		if err != nil {
			log.Println("failed to close http.Client, err: ", err)
		}
	}()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP Request failed with code %d", res.StatusCode)
	}

	bts, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(bts, data); err != nil {
		return err
	}
	return nil
}

func (s *Snapshots) fetchAndDecodeSnapshotsStats() (map[string]SnapshotStatsResponse, error) {
	mssr := make(map[string]SnapshotStatsResponse)

	u := *s.url
	u.Path = path.Join(u.Path, "/_snapshot")
	var srr SnapshotRepositoriesResponse
	err := s.getAndParseURL(&u, &srr)
	if err != nil {
		return nil, err
	}
	for repository := range srr {
		u := *s.url
		u.Path = path.Join(u.Path, "/_snapshot", repository, "/_all")
		var ssr SnapshotStatsResponse
		err := s.getAndParseURL(&u, &ssr)
		if err != nil {
			continue
		}
		mssr[repository] = ssr
	}

	return mssr, nil
}

// Collect gets Snapshots metric values
func (s *Snapshots) Collect(ch chan<- prometheus.Metric) {

	// indices
	snapshotsStatsResp, err := s.fetchAndDecodeSnapshotsStats()
	if err != nil {
		log.Println("failed to fetch and decode snapshot stats, err: ", err)
		return
	}

	// Snapshots stats
	for repositoryName, snapshotStats := range snapshotsStatsResp {
		for _, metric := range s.repositoryMetrics {
			ch <- prometheus.MustNewConstMetric(
				metric.Desc,
				metric.Type,
				metric.Value(snapshotStats),
				metric.Labels(repositoryName)...,
			)
		}
		if len(snapshotStats.Snapshots) == 0 {
			continue
		}

		lastSnapshot := snapshotStats.Snapshots[len(snapshotStats.Snapshots)-1]
		for _, metric := range s.snapshotMetrics {
			ch <- prometheus.MustNewConstMetric(
				metric.Desc,
				metric.Type,
				metric.Value(lastSnapshot),
				metric.Labels(repositoryName, lastSnapshot)...,
			)
		}
	}
}
