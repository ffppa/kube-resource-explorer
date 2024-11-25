package kube

import (
	"context"
	"fmt"
	"sort"
	"time"

	log "github.com/Sirupsen/logrus"
	promapi "github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	k8sv1 "k8s.io/api/core/v1"
)

type PrometheusClient struct {
	ctx     context.Context
	client  v1.API
	address string
}

func NewPrometheusClient(address string) (*PrometheusClient, error) {
	client, err := promapi.NewClient(promapi.Config{
		Address: address,
	})
	if err != nil {
		log.Fatal("Failed to create Prometheus client: %s", err)
	}

	return &PrometheusClient{
		ctx:     context.Background(),
		client:  v1.NewAPI(client),
		address: address,
	}, nil
}

type MetricJob struct {
	ContainerName string
	PodName       string
	PodUID        string
	Duration      time.Duration
	MetricType    k8sv1.ResourceName
	jobs          <-chan *MetricJob
	collector     chan<- *ContainerMetrics
}

func sortPointsAsc(points model.Matrix) {
	sort.Slice(points, func(i, j int) bool {
		return points[i].Values[0].Timestamp.Before(points[j].Values[0].Timestamp)
	})
}

func evaluateMemMetrics(matrix model.Matrix) *ContainerMetrics {
	var data []int64
	for _, sampleStream := range matrix {
		for _, sample := range sampleStream.Values {
			data = append(data, int64(sample.Value))
		}
	}
	if len(data) == 0 {
		log.Warningf("Memory metrics data is empty, skipping evaluation.")
		return nil
	}

	sortPointsAsc(matrix)
	min, max := MinMax_int64(data)
	return &ContainerMetrics{
		MetricType: k8sv1.ResourceMemory,
		MemoryLast: NewMemoryResource(data[len(data)-1]),
		MemoryMin:  NewMemoryResource(min),
		MemoryMax:  NewMemoryResource(max),
		MemoryMode: NewMemoryResource(int64(len(data))),
		DataPoints: int64(len(data)),
	}
}

func evaluateCpuMetrics(matrix model.Matrix) *ContainerMetrics {
	var data []int64
	for _, sampleStream := range matrix {
		for i := 1; i < len(sampleStream.Values); i++ {
			cur := sampleStream.Values[i]
			prev := sampleStream.Values[i-1]
			interval := cur.Timestamp.Sub(prev.Timestamp).Seconds()
			delta := float64(cur.Value) - float64(prev.Value)
			data = append(data, int64((delta/interval)*1000))
		}
	}
	if len(data) == 0 {
		log.Warningf("CPU metrics data is empty, skipping evaluation.")
		return nil // o qualunque valore tu voglia restituire in questo caso
	}

	sortPointsAsc(matrix)
	min, max := MinMax_int64(data)
	return &ContainerMetrics{
		MetricType: k8sv1.ResourceCPU,
		CpuLast:    NewCpuResource(data[len(data)-1]),
		CpuMin:     NewCpuResource(min),
		CpuMax:     NewCpuResource(max),
		CpuAvg:     NewCpuResource(int64(average_int64(data))),
		DataPoints: int64(len(data)),
	}
}

// ListMetrics function modified to fit the right query expression.
func (p *PrometheusClient) ListMetrics(metricName, containerName, podName string, duration time.Duration) model.Matrix {
	end := time.Now()
	start := end.Add(-duration)
	// Modification: Changed query from range vector to instant vector
	query := fmt.Sprintf("%s{container=\"%s\", pod=\"%s\"}", metricName, containerName, podName)

	result, warnings, err := p.client.QueryRange(p.ctx, query, v1.Range{
		Start: start,
		End:   end,
		Step:  time.Minute,
	})

	if err != nil {
		log.WithError(err).Error("querying Prometheus")
		return nil
	}

	if len(warnings) > 0 {
		log.Warn(warnings)
	}

	return result.(model.Matrix)
}

func (p *PrometheusClient) ContainerMetrics(containerName, podName string, duration time.Duration, metricType k8sv1.ResourceName) *ContainerMetrics {
	var m *ContainerMetrics
	switch metricType {
	case k8sv1.ResourceCPU:
		matrix := p.ListMetrics("container_cpu_usage_seconds_total", containerName, podName, duration)
		m = evaluateCpuMetrics(matrix)
	case k8sv1.ResourceMemory:
		matrix := p.ListMetrics("container_memory_usage_bytes", containerName, podName, duration)
		m = evaluateMemMetrics(matrix)
	}
	if m == nil {
		return nil
	}
	m.ContainerName = containerName
	return m
}

func (p *PrometheusClient) Run(jobs chan<- *MetricJob, collector <-chan *ContainerMetrics, pods []k8sv1.Pod, duration time.Duration, metricType k8sv1.ResourceName) (metrics []*ContainerMetrics) {
	go func() {
		for _, pod := range pods {
			for _, container := range pod.Spec.Containers {
				jobs <- &MetricJob{
					ContainerName: container.Name,
					PodName:       pod.GetName(),
					PodUID:        string(pod.ObjectMeta.UID),
					Duration:      duration,
					MetricType:    metricType,
				}
			}
		}
		close(jobs)
	}()
	for job := range collector {
		metrics = append(metrics, job)
	}
	return
}

func (p *PrometheusClient) Worker(jobs <-chan *MetricJob, collector chan<- *ContainerMetrics) {
	for job := range jobs {
		m := p.ContainerMetrics(job.ContainerName, job.PodName, job.Duration, job.MetricType)
		if m == nil {
			continue
		}
		m.PodName = job.PodName
		collector <- m
	}
	close(collector)
}

func (k *KubeClient) Historical(promAddress string, ctx context.Context, namespace string, resourceName k8sv1.ResourceName, duration time.Duration, sort string, reverse bool, csv bool, advise bool) {
	promClient, err := NewPrometheusClient(promAddress)
	if err != nil {
		panic(err.Error())
	}

	activePods, err := k.ActivePods(ctx, namespace, "")
	if err != nil {
		panic(err.Error())
	}
	log.Infof("Found %d active pods\n", len(activePods))
	jobs := make(chan *MetricJob)
	collector := make(chan *ContainerMetrics)
	go promClient.Worker(jobs, collector)
	metrics := promClient.Run(jobs, collector, activePods, duration, resourceName)
	rows, dataPoints := FormatContainerMetrics(metrics, resourceName, duration, sort, reverse, advise)

	if csv {
		prefix := "kube-resource-usage"
		if namespace == "" {
			prefix += "-all"
		} else {
			prefix += fmt.Sprintf("-%s", namespace)
		}

		filename := ExportCSV(prefix, rows)
		fmt.Printf("Exported %d rows to %s\n", len(rows), filename)
	} else {
		PrintContainerMetrics(rows, duration, dataPoints)
	}
}
