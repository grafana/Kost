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
)

// ErrUnknownKind is the error throw when the kind of the resource in
// the manifest is unknown to the parser.
var ErrUnknownKind = errors.New("unknown kind")

// Requirements holds the per-pod resource requirements parsed from a manifest,
// plus the replica count needed to compute aggregate cost.
// CPUPerPod is in millicores; MemoryPerPod and PersistentVolumePerPod are in bytes.
// Each PerPod field is the sum across all containers (or PVC templates) in a single pod.
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
		containers []corev1.Container
		replicas   = 1
	)

	switch x := obj.(type) {
	case *appsv1.StatefulSet:
		containers = x.Spec.Template.Spec.Containers
		if x.Spec.Replicas != nil {
			replicas = int(*x.Spec.Replicas)
		}
		addPersistentVolumeClaimRequirements(x.Spec.VolumeClaimTemplates, &r)

	case *appsv1.Deployment:
		containers = x.Spec.Template.Spec.Containers
		if x.Spec.Replicas != nil {
			replicas = int(*x.Spec.Replicas)
		}

	case *appsv1.DaemonSet:
		containers = x.Spec.Template.Spec.Containers
		// DaemonSets don't have a replica count, so we need to use the number of nodes in the cluster.
		if costModel == nil {
			return r, fmt.Errorf("%w: daemonsets require a cost model", ErrUnknownKind)
		}
		if costModel.Cluster != nil && costModel.Cluster.NodeCount > replicas {
			replicas = costModel.Cluster.NodeCount
		}

	case *batchv1.Job:
		containers = x.Spec.Template.Spec.Containers

	case *batchv1.CronJob:
		containers = x.Spec.JobTemplate.Spec.Template.Spec.Containers

	case *corev1.Pod:
		containers = x.Spec.Containers

	default:
		return r, fmt.Errorf("%w: %v (%T)", ErrUnknownKind, kind, x)
	}

	r.Kind = kind.Kind
	r.Replicas = replicas
	addContainersRequirements(containers, &r)
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

// addContainersRequirements adds per-pod container CPU and memory to the given requirements.
func addContainersRequirements(containers []corev1.Container, r *Requirements) {
	for _, container := range containers {
		resources := container.Resources
		r.CPUPerPod += resources.Requests.Cpu().MilliValue()
		r.MemoryPerPod += resources.Requests.Memory().Value()
	}
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
