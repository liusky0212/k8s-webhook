package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
)

// EnvVar names
const (
	NamespaceEnvVar     = "NAMESPACE"
	LabelSelectorEnvVar = "LABEL_SELECTOR"
	CPULimitEnvVar      = "CPU_LIMIT"
	MemoryLimitEnvVar   = "MEMORY_LIMIT"
	CPURequestEnvVar    = "CPU_REQUEST"
	MemoryRequestEnvVar = "MEMORY_REQUEST"
)

// Get env var values
var (
	namespace     = os.Getenv(NamespaceEnvVar)
	labelSelector = os.Getenv(LabelSelectorEnvVar)
	cpuLimit      = os.Getenv(CPULimitEnvVar)
	memoryLimit   = os.Getenv(MemoryLimitEnvVar)
	cpuRequest    = os.Getenv(CPURequestEnvVar)
	memoryRequest = os.Getenv(MemoryRequestEnvVar)
)

func mutatePod(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusBadRequest)
		return
	}

	// Unmarshal the request
	admissionReview := admissionv1.AdmissionReview{}
	err = json.Unmarshal(body, &admissionReview)
	if err != nil {
		http.Error(w, "Error unmarshalling request", http.StatusBadRequest)
		return
	}

	// Check if the namespace matches
	if admissionReview.Request.Namespace != namespace {
		w.WriteHeader(http.StatusOK)
		return
	}

	pod := corev1.Pod{}
	err = json.Unmarshal(admissionReview.Request.Object.Raw, &pod)
	if err != nil {
		http.Error(w, "Error unmarshalling pod", http.StatusBadRequest)
		return
	}

	// Check if the pod labels match the label selector
	selector, err := metav1.ParseToLabelSelector(labelSelector)
	if err != nil {
		http.Error(w, "Error parsing label selector", http.StatusBadRequest)
		return
	}

	labelSetSelector, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		http.Error(w, "Error converting label selector to selector", http.StatusBadRequest)
		return
	}

	if !labelSetSelector.Matches(labels.Set(pod.Labels)) {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Apply resources limits and requests
	applyResourceLimitsAndRequests(&pod)

	patchType := admissionv1.PatchTypeJSONPatch
	admissionResponse := admissionv1.AdmissionResponse{
		UID:       admissionReview.Request.UID,
		Allowed:   true,
		PatchType: &patchType,
	}
	admissionReview.Response = &admissionResponse

	responseBody, err := json.Marshal(admissionReview)
	if err != nil {
		http.Error(w, "Error marshalling response", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write(responseBody)
}

func applyResourceLimitsAndRequests(pod *corev1.Pod) {
	for i := range pod.Spec.Containers {
		container := &pod.Spec.Containers[i]

		if container.Resources.Limits == nil {
			container.Resources.Limits = corev1.ResourceList{}
		}
		if container.Resources.Requests == nil {
			container.Resources.Requests = corev1.ResourceList{}
		}
		// Apply CPU and memory limits
		if cpuLimit != "" {
			if _, ok := container.Resources.Limits[corev1.ResourceCPU]; !ok {
				cpuLimitQuantity, err := strconv.ParseInt(cpuLimit, 10, 64)
				if err == nil {
					container.Resources.Limits[corev1.ResourceCPU] = *resource.NewMilliQuantity(cpuLimitQuantity, resource.DecimalSI)
				}
			}
		}

		if memoryLimit != "" {
			if _, ok := container.Resources.Limits[corev1.ResourceMemory]; !ok {
				memoryLimitQuantity, err := strconv.ParseInt(memoryLimit, 10, 64)
				if err == nil {
					container.Resources.Limits[corev1.ResourceMemory] = *resource.NewQuantity(memoryLimitQuantity, resource.BinarySI)
				}
			}
		}

		// Apply CPU and memory requests
		if cpuRequest != "" {
			if _, ok := container.Resources.Requests[corev1.ResourceCPU]; !ok {
				cpuRequestQuantity, err := strconv.ParseInt(cpuRequest, 10, 64)
				if err == nil {
					container.Resources.Requests[corev1.ResourceCPU] = *resource.NewMilliQuantity(cpuRequestQuantity, resource.DecimalSI)
				}
			}
		}

		if memoryRequest != "" {
			if _, ok := container.Resources.Requests[corev1.ResourceMemory]; !ok {
				memoryRequestQuantity, err := strconv.ParseInt(memoryRequest, 10, 64)
				if err == nil {
					container.Resources.Requests[corev1.ResourceMemory] = *resource.NewQuantity(memoryRequestQuantity, resource.BinarySI)
				}
			}
		}
	}
}

func main() {
	http.HandleFunc("/mutate-pod", mutatePod)
	port := "8080"
	fmt.Printf("Listening on :%s...\n", port)
	if err := http.ListenAndServeTLS(":"+port, "/tls/tls.crt", "/tls/tls.key", nil); err != nil {
		panic(err)
	}
}
