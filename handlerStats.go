package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"net/http"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	resourcehelper "k8s.io/kubectl/pkg/util/resource"
)

type NodeReport struct {
	corev1.Node
	Pods   []corev1.Pod
	Reqs   map[string]corev1.ResourceList
	Limits map[string]corev1.ResourceList
}

func handlerStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/javascript")

	reports, err := getNodesReport(clientset)
	if err != nil {
		http.Error(w, errors.Wrap(err, "get nodes report").Error(), 400)
		return
	}

	out, err := createCSV(reports)
	if err != nil {
		http.Error(w, errors.Wrap(err, "create CSV report").Error(), 400)
		return
	}

	w.Write(out)
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

func createCSV(reports []NodeReport) ([]byte, error) {
	buf := new(bytes.Buffer)
	w := csv.NewWriter(buf)

	if err := w.Write([]string{"node", "pod", "cpu_req"}); err != nil {
		return nil, err
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
				return nil, err
			}
		}
	}

	w.Flush()

	if err := w.Error(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
