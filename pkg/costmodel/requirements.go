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

// Requirements is a struct that holds the aggergated amount of resources for a given manifest file.
// CPU and Memory are in millicores and bytes respectively and are the sum of all containers in a pod.
// TODO: Calculate the amount of persistent volume required for a given manifest. This will require finding the associated PVC and calculating the size of the volume.
type Requirements struct {
	CPU              int64
	Memory           int64
	PersistentVolume int64
	Kind             string
	Namespace        string
	Name             string
}

// AddRequirements will increment the resources by the amount specified in the given requirements.
func (r *Requirements) AddRequirements(reqs corev1.ResourceRequirements) {
	r.CPU += reqs.Requests.Cpu().MilliValue()
	r.Memory += reqs.Requests.Memory().Value()
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
		addPersistentVolumeClaimRequirements(x.Spec.VolumeClaimTemplates, &r, replicas)

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
	addContainersRequirements(containers, &r, replicas)
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

// addPersistentVolumeClaimRequirements will add the resources requested by the given persistent volume claims to the given requirements.
func addPersistentVolumeClaimRequirements(templates []corev1.PersistentVolumeClaim, r *Requirements, replicas int) {
	for _, template := range templates {
		r.PersistentVolume += int64(replicas) * template.Spec.Resources.Requests.Storage().Value()
	}
}

// addContainersRequirements will add the resources requested by the given containers to the given requirements.
// If the number of replicas is greater than 1, the resources will be multiplied by the number of replicas.
func addContainersRequirements(containers []corev1.Container, r *Requirements, replicas int) {
	for _, container := range containers {
		resources := container.Resources
		r.CPU += resources.Requests.Cpu().MilliValue() * int64(replicas)
		r.Memory += resources.Requests.Memory().Value() * int64(replicas)
	}
}

// Delta returns the difference between two resources.
// A positive value signals that the resource has increased.
// A negative value signals that the resource has decreased.
func Delta(from, to Requirements) Requirements {
	return Requirements{
		CPU:              to.CPU - from.CPU,
		Memory:           to.Memory - from.Memory,
		PersistentVolume: to.PersistentVolume - from.PersistentVolume,
	}
}
