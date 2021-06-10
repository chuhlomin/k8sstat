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

	w.Header().Add("Content-Type", "text/csv")
	w.Write(out)
}

func getNodesReport(clientset *kubernetes.Clientset) ([]NodeReport, error) {
	var reports []NodeReport

	nodes, err := clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "get nodes")
	}

	for _, node := range nodes.Items {
		pods, err := clientset.CoreV1().Pods("").List(
			context.TODO(),
			metav1.ListOptions{
				FieldSelector: "spec.nodeName=" + node.ObjectMeta.Name,
			},
		)
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

	if err := w.Write(
		[]string{
			"node",
			"pod",
			"cpu_req",
			"cpu_lim",
			"mem_req",
			"mem_lim",
			"pod_status",
			"namespace",
		},
	); err != nil {
		return nil, err
	}

	for _, node := range reports {
		allocatable := node.Status.Capacity
		if len(node.Status.Allocatable) > 0 {
			allocatable = node.Status.Allocatable
		}

		for _, pod := range node.Pods {
			cpuReq, cpuLimit, memoryReq, memoryLimit := node.Reqs[pod.Name][corev1.ResourceCPU],
				node.Limits[pod.Name][corev1.ResourceCPU],
				node.Reqs[pod.Name][corev1.ResourceMemory],
				node.Limits[pod.Name][corev1.ResourceMemory]
			fractionCPUReq := float64(cpuReq.MilliValue()) / float64(allocatable.Cpu().MilliValue()) * 100
			fractionCPULimit := float64(cpuLimit.MilliValue()) / float64(allocatable.Cpu().MilliValue()) * 100
			fractionMemoryReq := float64(memoryReq.Value()) / float64(allocatable.Memory().Value()) * 100
			fractionMemoryLimit := float64(memoryLimit.Value()) / float64(allocatable.Memory().Value()) * 100

			if err := w.Write([]string{
				node.Node.ObjectMeta.Name,
				pod.ObjectMeta.Name,
				fmt.Sprintf("%d", int64(fractionCPUReq)*10),
				fmt.Sprintf("%d", int64(fractionCPULimit)*10),
				fmt.Sprintf("%d", int64(fractionMemoryReq)*10),
				fmt.Sprintf("%d", int64(fractionMemoryLimit)*10),
				string(pod.Status.Phase),
				pod.Namespace,
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
