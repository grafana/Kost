package costmodel

import (
	"errors"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	resourcehelper "k8s.io/component-helpers/resource"
)

// ErrUnknownKind is the error throw when the kind of the resource in
// the manifest is unknown to the parser.
var ErrUnknownKind = errors.New("unknown kind")

// Requirements holds the per-pod resource requirements parsed from a manifest,
// plus the replica count needed to compute aggregate cost.
// CPUPerPod is in millicores; MemoryPerPod and PersistentVolumePerPod are in bytes.
// CPU and memory are effective scheduling requests, including init containers and Pod overhead.
// PersistentVolumePerPod is the sum across PVC templates in a single pod.
// Use TotalCPU / TotalMemory / TotalPersistentVolume to get aggregate values across replicas.
type Requirements struct {
	CPUPerPod              int64
	MemoryPerPod           int64
	PersistentVolumePerPod int64
	Replicas               int
	Kind                   string
	Namespace              string
	Name                   string
}

// AddRequirements increments the per-pod resources by the amount specified.
func (r *Requirements) AddRequirements(reqs corev1.ResourceRequirements) {
	r.CPUPerPod += reqs.Requests.Cpu().MilliValue()
	r.MemoryPerPod += reqs.Requests.Memory().Value()
}

// TotalCPU returns aggregate CPU (millicores) across all replicas.
func (r Requirements) TotalCPU() int64 {
	return r.CPUPerPod * int64(r.Replicas)
}

// TotalMemory returns aggregate memory (bytes) across all replicas.
func (r Requirements) TotalMemory() int64 {
	return r.MemoryPerPod * int64(r.Replicas)
}

// TotalPersistentVolume returns aggregate persistent volume (bytes) across all replicas.
func (r Requirements) TotalPersistentVolume() int64 {
	return r.PersistentVolumePerPod * int64(r.Replicas)
}

// ParseManifest will parse a manifest file and return the aggregated amount of resources requested.
// The manifest can be a Deployment, StatefulSet, DaemonSet, Cronjob, Job, or Pod.
// If the manifest has the number of Replicas, the total resources will be multiplied by the number of replicas.
func ParseManifest(src []byte, costModel *CostModel) (Requirements, error) {
	var r Requirements

	decode := scheme.Codecs.UniversalDeserializer().Decode

	obj, kind, err := decode(src, nil, nil)
	if err != nil {
		return r, fmt.Errorf("%w: could not decode object: %s", ErrUnknownKind, err)
	}

	var (
		podSpec  *corev1.PodSpec
		replicas = 1
	)

	switch x := obj.(type) {
	case *appsv1.StatefulSet:
		podSpec = &x.Spec.Template.Spec
		if x.Spec.Replicas != nil {
			replicas = int(*x.Spec.Replicas)
		}
		addPersistentVolumeClaimRequirements(x.Spec.VolumeClaimTemplates, &r)

	case *appsv1.Deployment:
		podSpec = &x.Spec.Template.Spec
		if x.Spec.Replicas != nil {
			replicas = int(*x.Spec.Replicas)
		}

	case *appsv1.DaemonSet:
		podSpec = &x.Spec.Template.Spec
		// DaemonSets don't have a replica count, so we need to use the number of nodes in the cluster.
		if costModel == nil {
			return r, fmt.Errorf("%w: daemonsets require a cost model", ErrUnknownKind)
		}
		if costModel.Cluster != nil && costModel.Cluster.NodeCount > replicas {
			replicas = costModel.Cluster.NodeCount
		}

	case *batchv1.Job:
		podSpec = &x.Spec.Template.Spec

	case *batchv1.CronJob:
		podSpec = &x.Spec.JobTemplate.Spec.Template.Spec

	case *corev1.Pod:
		podSpec = &x.Spec

	default:
		return r, fmt.Errorf("%w: %v (%T)", ErrUnknownKind, kind, x)
	}

	r.Kind = kind.Kind
	r.Replicas = replicas
	addPodRequirements(podSpec, &r)
	err = addMetadataToRequirements(obj, &r)
	if err != nil {
		return r, err
	}
	return r, nil
}

func addMetadataToRequirements(obj runtime.Object, requirements *Requirements) error {
	metadata, err := meta.Accessor(obj)
	if err != nil {
		return err
	}
	requirements.Namespace = metadata.GetNamespace()
	requirements.Name = metadata.GetName()
	return nil
}

// addPersistentVolumeClaimRequirements adds per-pod PVC storage to the given requirements.
func addPersistentVolumeClaimRequirements(templates []corev1.PersistentVolumeClaim, r *Requirements) {
	for _, template := range templates {
		r.PersistentVolumePerPod += template.Spec.Resources.Requests.Storage().Value()
	}
}

// addPodRequirements adds effective per-pod CPU and memory requests.
func addPodRequirements(spec *corev1.PodSpec, r *Requirements) {
	// Manifest decoding skips API-server defaulting: missing requests inherit limits when the Pod is created.
	for _, containers := range [][]corev1.Container{spec.Containers, spec.InitContainers} {
		for i := range containers {
			resources := &containers[i].Resources
			for name, limit := range resources.Limits {
				if _, exists := resources.Requests[name]; !exists {
					if resources.Requests == nil {
						resources.Requests = corev1.ResourceList{}
					}
					resources.Requests[name] = limit.DeepCopy()
				}
			}
		}
	}
	pod := &corev1.Pod{Spec: *spec}
	if spec.Resources != nil && len(spec.Resources.Limits) > 0 {
		// Match Kubernetes Pod defaulting: container requests take precedence over Pod limits.
		containerRequests := resourcehelper.AggregateContainerRequests(pod, resourcehelper.PodResourcesOptions{})
		for _, name := range []corev1.ResourceName{corev1.ResourceCPU, corev1.ResourceMemory} {
			if _, exists := spec.Resources.Requests[name]; exists {
				continue
			}
			request, exists := containerRequests[name]
			if !exists {
				request, exists = spec.Resources.Limits[name]
			}
			if exists {
				if spec.Resources.Requests == nil {
					spec.Resources.Requests = corev1.ResourceList{}
				}
				spec.Resources.Requests[name] = request.DeepCopy()
			}
		}
	}
	requests := resourcehelper.PodRequests(pod, resourcehelper.PodResourcesOptions{})
	r.CPUPerPod += requests.Cpu().MilliValue()
	r.MemoryPerPod += requests.Memory().Value()
}

// Delta returns the field-wise difference between two resources.
// A positive value signals that the resource has increased.
// A negative value signals that the resource has decreased.
func Delta(from, to Requirements) Requirements {
	return Requirements{
		CPUPerPod:              to.CPUPerPod - from.CPUPerPod,
		MemoryPerPod:           to.MemoryPerPod - from.MemoryPerPod,
		PersistentVolumePerPod: to.PersistentVolumePerPod - from.PersistentVolumePerPod,
		Replicas:               to.Replicas - from.Replicas,
	}
}
