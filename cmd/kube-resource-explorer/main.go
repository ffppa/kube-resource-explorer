package main

import (
	"context"
	"flag"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ffppa/kube-resource-explorer/pkg/kube"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/tools/clientcmd"
)

var GitCommit string

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}

func main() {
	log.SetLevel(log.InfoLevel)
	log.SetOutput(os.Stdout)

	default_duration, err := time.ParseDuration("4h")
	if err != nil {
		panic(err.Error())
	}

	var (
		namespace            = flag.String("namespace", "", "filter by namespace (defaults to all)")
		sort                 = flag.String("sort", "CpuReq", "field to sort by")
		reverse              = flag.Bool("reverse", false, "reverse sort output")
		historical           = flag.Bool("historical", false, "show historical info")
		duration             = flag.Duration("duration", default_duration, "specify the duration")
		mem_only             = flag.Bool("mem", false, "show historical memory info")
		cpu_only             = flag.Bool("cpu", false, "show historical cpu info")
		prometheus_namespace = flag.String("prometheus_namespace", "monitoring", "select the prometheus namespace")
		csv                  = flag.Bool("csv", false, "Export results to csv file")
		kubeconfig           *string
	)

	if kubeenv := os.Getenv("KUBECONFIG"); kubeenv != "" {
		kubeconfig = flag.String("kubeconfig", kubeenv, "absolute path to the kubeconfig file")
	} else if home := homeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}

	flag.Parse()

	ctx := context.Background()

	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	k := kube.NewKubeClient(clientset)

	if *historical {

		promCurrentPod, err := k.GetCurrentPrometheusPod(*prometheus_namespace)
		if err != nil {
			panic(err.Error())
		}

		promAddress, err := k.PrometheusForwarder(config, promCurrentPod, *prometheus_namespace)
		if err != nil {
			log.Errorf("Error starting prometheus forwarder: %s", err)
			os.Exit(1)
		}

		m := kube.ContainerMetrics{}

		if !m.Validate(*sort) {
			fmt.Printf("\"%s\" is not a valid field. Possible values are:\n\n%s\n", *sort, strings.Join(kube.GetFields(&m), ", "))
			os.Exit(1)
		}

		var resourceName v1.ResourceName

		if *mem_only {
			resourceName = v1.ResourceMemory
		} else if *cpu_only {
			resourceName = v1.ResourceCPU
		} else {
			log.Errorf("Unknown metric type, please specify -mem or -cpu")
			log.Exit(2)
		}

		k.Historical(promAddress, ctx, *namespace, resourceName, *duration, *sort, *reverse, *csv)

	} else {

		r := kube.ContainerResources{}

		if !r.Validate(*sort) {
			fmt.Printf("\"%s\" is not a valid field. Possible values are:\n\n%s\n", *sort, strings.Join(kube.GetFields(r), ", "))
			os.Exit(1)
		}

		k.ResourceUsage(ctx, *namespace, *sort, *reverse, *csv)
	}
}
