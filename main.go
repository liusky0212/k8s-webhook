package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

var (
	runtimeScheme = runtime.NewScheme()
	codecs        = serializer.NewCodecFactory(runtimeScheme)
	deserializer  = codecs.UniversalDeserializer()
)

func main() {
	http.HandleFunc("/mutate", mutateHandler)
	server := &http.Server{
		Addr:    ":8443",
		Handler: http.DefaultServeMux,
	}

	fmt.Println("Starting webhook server...")
	err := server.ListenAndServeTLS("server.crt", "server.key")
	if err != nil {
		panic(err)
	}
}

func mutateHandler(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}

	var admissionReview admissionv1.AdmissionReview
	if _, _, err := deserializer.Decode(body, nil, &admissionReview); err != nil {
		http.Error(w, "Error decoding request", http.StatusBadRequest)
		return
	}

	response := processAdmissionReview(admissionReview)
	respBytes, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "Error marshalling response", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(respBytes)
}

func processAdmissionReview(admissionReview admissionv1.AdmissionReview) *admissionv1.AdmissionReview {
	namespace := os.Getenv("TARGET_NAMESPACE")
	targetLabelKey := os.Getenv("TARGET_LABEL_KEY")
	targetLabelValue := os.Getenv("TARGET_LABEL_VALUE")
	cpuRequest := os.Getenv("CPU_REQUEST")
	cpuLimit := os.Getenv("CPU_LIMIT")
	memoryRequest := os.Getenv("MEMORY_REQUEST")
	memoryLimit := os.Getenv("MEMORY_LIMIT")

	pod := &corev1.Pod{}
	if err := json.Unmarshal(admissionReview.Request.Object.Raw, pod); err != nil {
		return &admissionv1.AdmissionReview{
			Response: &admissionv1.AdmissionResponse{
				UID:     admissionReview.Request.UID,
				Allowed: false,
				Result: &metav1.Status{
					Message: fmt.Sprintf("Error unmarshalling pod: %v", err),
				},
			},
		}
	}

	if pod.Namespace != namespace || pod.Labels[targetLabelKey] != targetLabelValue {
		return &admissionv1.AdmissionReview{
			Response: &admissionv1.AdmissionResponse{
				UID:     admissionReview.Request.UID,
				Allowed: true,
			},
		}
	}

	patch, err := json.Marshal(createPatch(pod, cpuRequest, cpuLimit, memoryRequest, memoryLimit))
	if err != nil {
		return &admissionv1.AdmissionReview{
			Response: &admissionv1.AdmissionResponse{
				UID:     admissionReview.Request.UID,
				Allowed: false,
				Result: &metav1.Status{
					Message: fmt.Sprintf("Error marshalling patch: %v", err),
				},
			},
		}
	}
	return &admissionv1.AdmissionReview{
		Response: &admissionv1.AdmissionResponse{
			UID:       admissionReview.Request.UID,
			Allowed:   true,
			Patch:     patch,
			PatchType: func() *admissionv1.PatchType { pt := admissionv1.PatchTypeJSONPatch; return &pt }(),
		},
	}
}

func createPatch(pod *corev1.Pod, cpuRequest, cpuLimit, memoryRequest, memoryLimit string) []map[string]interface{} {
	var patch []map[string]interface{}
	for i := range pod.Spec.Containers {
		container := &pod.Spec.Containers[i]
		if container.Resources.Requests == nil {
			container.Resources.Requests = corev1.ResourceList{}
		}
		if container.Resources.Limits == nil {
			container.Resources.Limits = corev1.ResourceList{}
		}

		if cpuRequest != "" {
			patch = append(patch, map[string]interface{}{
				"op":    "add",
				"path":  fmt.Sprintf("/spec/containers/%d/resources/requests/cpu", i),
				"value": cpuRequest,
			})
		}
		if cpuLimit != "" {
			patch = append(patch, map[string]interface{}{
				"op":    "add",
				"path":  fmt.Sprintf("/spec/containers/%d/resources/limits/cpu", i),
				"value": cpuLimit,
			})
		}
		if memoryRequest != "" {
			patch = append(patch, map[string]interface{}{
				"op":    "add",
				"path":  fmt.Sprintf("/spec/containers/%d/resources/requests/memory", i),
				"value": memoryRequest,
			})
		}
		if memoryLimit != "" {
			patch = append(patch, map[string]interface{}{
				"op":    "add",
				"path":  fmt.Sprintf("/spec/containers/%d/resources/limits/memory", i),
				"value": memoryLimit,
			})
		}
	}

	return patch
}
