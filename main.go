package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	resourcehelper "k8s.io/kubectl/pkg/util/resource"

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

func main() {
	log.Println("Starting...")

	if err := run(); err != nil {
		log.Printf("ERROR %v", err)
	}

	log.Println("Stopped")
}

type NodeReport struct {
	corev1.Node
	Pods   []corev1.Pod
	Reqs   map[string]corev1.ResourceList
	Limits map[string]corev1.ResourceList
}

func getNodesReport(clientset *kubernetes.Clientset) ([]NodeReport, error) {
	var reports []NodeReport

	nodes, err := clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "get nodes")
	}

	for _, node := range nodes.Items {
		pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{FieldSelector: "spec.nodeName=" + node.ObjectMeta.Name})
		if err != nil {
			return nil, errors.Wrap(err, "get pods")
		}

		report := NodeReport{
			Node:   node,
			Pods:   pods.Items,
			Reqs:   map[string]corev1.ResourceList{},
			Limits: map[string]corev1.ResourceList{},
		}

		for _, pod := range pods.Items {
			report.Reqs[pod.Name], report.Limits[pod.Name] = resourcehelper.PodRequestsAndLimits(&pod)
		}

		reports = append(reports, report)
	}

	return reports, nil
}

func writeCSV(reports []NodeReport) error {
	w := csv.NewWriter(os.Stdout)

	if err := w.Write([]string{"node", "pod", "cpu_req"}); err != nil {
		return err
	}

	for _, node := range reports {
		allocatable := node.Status.Capacity
		if len(node.Status.Allocatable) > 0 {
			allocatable = node.Status.Allocatable
		}

		for _, pod := range node.Pods {
			cpuReq := node.Reqs[pod.Name][corev1.ResourceCPU]
			fractionCPUReq := float64(cpuReq.MilliValue()) / float64(allocatable.Cpu().MilliValue()) * 100

			if err := w.Write([]string{
				node.Node.ObjectMeta.Name,
				pod.ObjectMeta.Name,
				fmt.Sprintf("%d", int64(fractionCPUReq)*10),
			}); err != nil {
				return err
			}
		}
	}

	w.Flush()

	if err := w.Error(); err != nil {
		return err
	}

	return nil
}

func run() error {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	log.Printf("Creating K8s client...")
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		return errors.Wrap(err, "build config")
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return errors.Wrap(err, "create client set from config")
	}

	log.Printf("Getting nodes report...")
	reports, err := getNodesReport(clientset)
	if err != nil {
		return errors.Wrap(err, "get node report")
	}

	log.Printf("Writing CSV...")
	if err := writeCSV(reports); err != nil {
		return errors.Wrap(err, "write CSV report")
	}

	return nil
}
