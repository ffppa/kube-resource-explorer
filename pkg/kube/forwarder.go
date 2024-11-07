package kube

import (
	"context"
	"fmt"
	log "github.com/Sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"net"
	"net/http"
	"net/url"
	"os"
)

func (k *KubeClient) PrometheusForwarder(config *rest.Config, prometheusPodName string, prometheusNamespace string) (string, error) {

	namespace := prometheusNamespace
	podName := prometheusPodName

	localPort, err := getFreePort()
	if err != nil {
		panic(err.Error())
	}

	stopCh := make(chan struct{}, 1)
	readyCh := make(chan struct{})
	go func() {
		err := prometheusPortForward(config, namespace, podName, localPort, "9090", stopCh, readyCh)
		if err != nil {
			panic(err.Error())
		}
	}()

	// Waiting forwarding ready
	<-readyCh

	// Trap signals
	//sigterm := make(chan os.Signal, 1)
	//signal.Notify(sigterm, os.Interrupt)
	//<-sigterm
	//close(stopCh)

	prometheusLocalAddress := fmt.Sprintf("http://localhost:%d", localPort)
	return prometheusLocalAddress, nil
}

func getFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer listener.Close()

	return listener.Addr().(*net.TCPAddr).Port, nil
}

func prometheusPortForward(config *rest.Config, namespace, podName string, localPort int, remotePort string, stopCh, readyCh chan struct{}) error {
	roundTripper, upgrader, err := spdy.RoundTripperFor(config)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", namespace, podName)
	uri, err := url.Parse(config.Host)
	if err != nil {
		log.Errorf("Error parsing URL: %v\n", err)
	}

	log.Infof("Connecting to %s", config.Host)
	serverURL := url.URL{
		Scheme: "https",
		Path:   path,
		Host:   uri.Host,
	}
	log.Infof("Connecting to %s", serverURL.String())

	ports := []string{fmt.Sprintf("%d:%s", localPort, remotePort)}

	pf, err := portforward.New(
		spdy.NewDialer(
			upgrader, &http.Client{Transport: roundTripper}, http.MethodPost, &serverURL,
		),
		ports,
		stopCh,
		readyCh,
		os.Stdout,
		os.Stderr,
	)

	if err != nil {
		return err
	}

	return pf.ForwardPorts()
}

func (k *KubeClient) GetCurrentPrometheusPod(namespace string) (string, error) {
	pods, err := k.clientset.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return "", err
	}

	for _, pod := range pods.Items {
		for _, container := range pod.Spec.Containers {
			if container.Name == "prometheus" { // TODO capire come selezionare dinamicamente il prometheus
				return pod.Name, nil
			}
		}
	}

	return "", fmt.Errorf("no pod with container %s found in namespace %s", "prometheus-server", namespace)
}
