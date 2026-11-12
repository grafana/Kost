package costmodel

import (
	"encoding/json"
	"os"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestParseManifest(t *testing.T) {
	pq := func(s string) *resource.Quantity {
		t.Helper()
		q, err := resource.ParseQuantity(s)
		if err != nil {
			t.Fatalf("parsing CPU %q: %v", s, err)
		}
		return &q
	}

	cpu := func(s string) int64 {
		t.Helper()
		return pq(s).MilliValue()
	}

	mem := func(s string) int64 {
		t.Helper()
		return pq(s).Value()
	}

	pv := func(s string) int64 {
		t.Helper()
		return pq(s).Value()
	}

	tests := map[string]Requirements{
		"Deployment": {
			CPUPerPod:    cpu("500m"),
			MemoryPerPod: mem("1000Mi"),
			Replicas:     1,
			Kind:         "Deployment",
			Namespace:    "opencost",
			Name:         "prom-label-proxy",
		},
		"Job": {
			CPUPerPod:    cpu("50m"),
			MemoryPerPod: mem("200Mi"),
			Replicas:     1,
			Kind:         "Job",
			Namespace:    "hosted-grafana",
			Name:         "hosted-grafana-source-ips-update-27973440",
		},

		"StatefulSet": {
			CPUPerPod:              cpu("1"),
			MemoryPerPod:           mem("4Gi"),
			PersistentVolumePerPod: pv("32Gi"),
			Replicas:               1,
			Kind:                   "StatefulSet",
			Namespace:              "opencost",
			Name:                   "opencost",
		},

		"DaemonSet": {
			CPUPerPod:    cpu("50m"),
			MemoryPerPod: mem("50Mi"),
			Replicas:     1,
			Kind:         "DaemonSet",
			Namespace:    "conntrack-exporter",
			Name:         "conntrack-exporter",
		},

		"Pod": {
			CPUPerPod:    cpu("45"),
			MemoryPerPod: mem("320Gi"),
			Replicas:     1,
			Kind:         "Pod",
			Namespace:    "default",
			Name:         "prometheus-0",
		},

		// Multi-container manifests
		"StatefulSet-with-2-containers": {
			CPUPerPod:              cpu("1") + cpu("10m"),
			MemoryPerPod:           mem("4Gi") + mem("55M"),
			PersistentVolumePerPod: pv("32Gi"),
			Replicas:               1,
			Kind:                   "StatefulSet",
			Namespace:              "opencost",
			Name:                   "opencost",
		},

		// With replicas
		"StatefulSet-with-replicas": {
			CPUPerPod:              cpu("45"),
			MemoryPerPod:           mem("320Gi"),
			PersistentVolumePerPod: pv("7500Gi"),
			Replicas:               2,
			Kind:                   "StatefulSet",
			Namespace:              "default",
			Name:                   "prometheus",
		},
	}

	for kind, exp := range tests {
		t.Run(kind, func(t *testing.T) {
			src, err := os.ReadFile("testdata/resource/" + kind + ".json")
			if err != nil {
				t.Fatalf("unexpected error reading manifest file: %v", err)
			}

			got, err := ParseManifest(src, &CostModel{})
			if err != nil {
				t.Fatalf("unexpected error parsing manifest: %v", err)
			}

			if exp != got {
				t.Fatalf("wrong parsed values:\nexp: %#v\ngot: %#v", exp, got)
			}
		})
	}

	t.Run("panics on StatefulSet without replicas", func(t *testing.T) {
		// This was first reported on Slack and it was failing all
		// Alertmanager PRs, blocking auto-rollouts.
		// https://raintank-corp.slack.com/archives/C051ALUR9LG/p1681286565973609
		src, err := os.ReadFile("testdata/resource/StatefulSet-without-replicas.yaml")
		if err != nil {
			t.Fatalf("unexpected error reading manifest file: %v", err)
		}

		got, err := ParseManifest(src, &CostModel{})
		if err != nil {
			t.Fatalf("unexpected error parsing manifest: %v", err)
		}

		exp := Requirements{
			CPUPerPod:              cpu("200m"),
			MemoryPerPod:           mem("1Gi"),
			PersistentVolumePerPod: pv("100Gi"),
			Replicas:               1,
			Kind:                   "StatefulSet",
			Namespace:              "alertmanager",
			Name:                   "alertmanager",
		}

		if exp != got {
			t.Fatalf("wrong parsed values:\nexp: %#v\ngot: %#v", exp, got)
		}
	})

	t.Run("panics on Daemonset if costmodel is nil", func(t *testing.T) {
		src, err := os.ReadFile("testdata/resource/DaemonSet.json")
		if err != nil {
			t.Fatalf("unexpected error reading manifest file: %v", err)
		}

		_, err = ParseManifest(src, nil)
		if err == nil {
			t.Fatalf("expected error parsing manifest")
		}
	})
}

func TestDelta(t *testing.T) {
	tests := map[string]struct {
		from Requirements
		to   Requirements
		want Requirements
	}{
		"Two equal resources should result in all values being zero": {
			from: Requirements{
				CPUPerPod:    1,
				MemoryPerPod: 2,
			},
			to: Requirements{
				CPUPerPod:    1,
				MemoryPerPod: 2,
			},
			want: Requirements{
				CPUPerPod:    0,
				MemoryPerPod: 0,
			},
		},
		"Two resources with more resources should result in the correct positive delta": {
			from: Requirements{
				CPUPerPod:    1,
				MemoryPerPod: 2,
			},
			to: Requirements{
				CPUPerPod:    2,
				MemoryPerPod: 4,
			},
			want: Requirements{
				CPUPerPod:    1,
				MemoryPerPod: 2,
			},
		},
		"To resources with less resources should result in the correct negative delta": {
			from: Requirements{
				CPUPerPod:    2,
				MemoryPerPod: 4,
			},
			to: Requirements{
				CPUPerPod:    1,
				MemoryPerPod: 2,
			},
			want: Requirements{
				CPUPerPod:    -1,
				MemoryPerPod: -2,
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := Delta(test.from, test.to); got != test.want {
				t.Errorf("Delta() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestParseManifestPodRequests(t *testing.T) {
	container := func(cpu, memory string, sidecar bool) corev1.Container {
		c := corev1.Container{Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{}}}
		if cpu != "" {
			c.Resources.Requests[corev1.ResourceCPU] = resource.MustParse(cpu)
		}
		if memory != "" {
			c.Resources.Requests[corev1.ResourceMemory] = resource.MustParse(memory)
		}
		if sidecar {
			always := corev1.ContainerRestartPolicyAlways
			c.RestartPolicy = &always
		}
		return c
	}
	limitsOnly := container("", "", false)
	limitsOnly.Resources.Requests = nil
	limitsOnly.Resources.Limits = corev1.ResourceList{
		corev1.ResourceCPU: resource.MustParse("3"), corev1.ResourceMemory: resource.MustParse("6Gi"),
	}
	explicitRequests := limitsOnly.DeepCopy()
	explicitRequests.Resources.Requests = corev1.ResourceList{
		corev1.ResourceCPU: resource.MustParse("0"), corev1.ResourceMemory: resource.MustParse("1Gi"),
	}
	partialRequests := limitsOnly.DeepCopy()
	partialRequests.Resources.Requests = corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m")}
	initLimitsOnly := limitsOnly.DeepCopy()
	always := corev1.ContainerRestartPolicyAlways
	initLimitsOnly.RestartPolicy = &always
	startupLimitsOnly := limitsOnly.DeepCopy()
	startupLimitsOnly.Resources.Limits[corev1.ResourceCPU] = resource.MustParse("8")
	startupLimitsOnly.Resources.Limits[corev1.ResourceMemory] = resource.MustParse("1Gi")

	podLimits := &corev1.ResourceRequirements{Limits: corev1.ResourceList{
		corev1.ResourceCPU: resource.MustParse("2"), corev1.ResourceMemory: resource.MustParse("4Gi"),
	}}
	explicitPodRequests := podLimits.DeepCopy()
	explicitPodRequests.Requests = corev1.ResourceList{
		corev1.ResourceCPU: resource.MustParse("0"), corev1.ResourceMemory: resource.MustParse("3Gi"),
	}

	partialPodRequests := podLimits.DeepCopy()
	partialPodRequests.Requests = corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("0")}

	tests := []struct {
		name        string
		spec        corev1.PodSpec
		cpu, memory string
	}{
		{
			name: "application containers only",
			spec: corev1.PodSpec{Containers: []corev1.Container{container("4", "8Gi", false), container("500m", "1Gi", false)}},
			cpu:  "4500m", memory: "9Gi",
		},
		{
			name: "ordinary init containers run sequentially",
			spec: corev1.PodSpec{Containers: []corev1.Container{container("4", "8Gi", false)}, InitContainers: []corev1.Container{container("8", "1Gi", false), container("6", "12Gi", false)}},
			cpu:  "8", memory: "12Gi",
		},
		{
			name: "later sidecar does not overlap earlier init",
			spec: corev1.PodSpec{Containers: []corev1.Container{container("2", "4Gi", false)}, InitContainers: []corev1.Container{container("1", "2Gi", true), container("8", "1Gi", false), container("2", "3Gi", true)}},
			cpu:  "9", memory: "9Gi",
		},
		{
			name: "earlier sidecars overlap later init",
			spec: corev1.PodSpec{Containers: []corev1.Container{container("2", "4Gi", false)}, InitContainers: []corev1.Container{container("1", "2Gi", true), container("2", "3Gi", true), container("8", "1Gi", false)}},
			cpu:  "11", memory: "9Gi",
		},
		{
			name: "no requests or limits",
			spec: corev1.PodSpec{Containers: []corev1.Container{{}}, InitContainers: []corev1.Container{container("", "", true), {}}},
			cpu:  "0", memory: "0",
		},
		{
			name: "application sidecar and ordinary init requests default to limits",
			spec: corev1.PodSpec{
				Containers:     []corev1.Container{limitsOnly},
				InitContainers: []corev1.Container{*initLimitsOnly, *startupLimitsOnly},
			},
			cpu: "11", memory: "12Gi",
		},
		{
			name: "explicit requests including zero override limits",
			spec: corev1.PodSpec{Containers: []corev1.Container{*explicitRequests}},
			cpu:  "0", memory: "1Gi",
		},
		{
			name: "only missing resources default to limits",
			spec: corev1.PodSpec{Containers: []corev1.Container{*partialRequests}},
			cpu:  "500m", memory: "6Gi",
		},
		{
			name: "overhead is added after init peak",
			spec: corev1.PodSpec{Containers: []corev1.Container{container("4", "8Gi", false)}, InitContainers: []corev1.Container{container("8", "1Gi", false)}, Overhead: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("250m"), corev1.ResourceMemory: resource.MustParse("128Mi")}},
			cpu:  "8250m", memory: "8320Mi",
		},
		{
			name: "pod requests default to pod limits",
			spec: corev1.PodSpec{Containers: []corev1.Container{{}}, Resources: podLimits},
			cpu:  "2", memory: "4Gi",
		},
		{
			name: "pod requests prefer container requests per resource",
			spec: corev1.PodSpec{Containers: []corev1.Container{container("1", "", false)}, Resources: podLimits},
			cpu:  "1", memory: "4Gi",
		},
		{
			name: "pod defaults preserve explicit zero container requests",
			spec: corev1.PodSpec{Containers: []corev1.Container{container("0", "", false)}, Resources: podLimits},
			cpu:  "0", memory: "4Gi",
		},
		{
			name: "explicit pod requests including zero override defaults",
			spec: corev1.PodSpec{Containers: []corev1.Container{container("1", "1Gi", false)}, Resources: explicitPodRequests},
			cpu:  "0", memory: "3Gi",
		},
		{
			name: "partial pod requests default only missing resources",
			spec: corev1.PodSpec{Containers: []corev1.Container{container("1", "", false)}, Resources: partialPodRequests},
			cpu:  "0", memory: "4Gi",
		},
		{
			name: "pod defaults use container limits and init peak before overhead",
			spec: corev1.PodSpec{
				Containers:     []corev1.Container{*partialRequests},
				InitContainers: []corev1.Container{container("1", "2Gi", true), container("8", "1Gi", false)},
				Resources: &corev1.ResourceRequirements{Limits: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("10"), corev1.ResourceMemory: resource.MustParse("12Gi"),
				}},
				Overhead: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("250m"), corev1.ResourceMemory: resource.MustParse("128Mi")},
			},
			cpu: "9250m", memory: "8320Mi",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"}, Spec: tt.spec}
			src, err := json.Marshal(pod)
			if err != nil {
				t.Fatal(err)
			}
			got, err := ParseManifest(src, nil)
			if err != nil {
				t.Fatal(err)
			}
			cpu, memory := resource.MustParse(tt.cpu), resource.MustParse(tt.memory)
			if got.CPUPerPod != cpu.MilliValue() || got.MemoryPerPod != memory.Value() {
				t.Fatalf("got CPU %dm, memory %d bytes; want CPU %s, memory %s", got.CPUPerPod, got.MemoryPerPod, tt.cpu, tt.memory)
			}
		})
	}
}

func TestParseManifestInitContainersWorkloadKinds(t *testing.T) {
	src, err := os.ReadFile("testdata/resource/StatefulSet-with-init-containers.yaml")
	if err != nil {
		t.Fatal(err)
	}
	obj, _, err := scheme.Codecs.UniversalDeserializer().Decode(src, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	statefulSet := obj.(*appsv1.StatefulSet)
	template := statefulSet.Spec.Template
	replicas := int32(3)
	objects := []runtime.Object{
		statefulSet,
		&appsv1.Deployment{TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"}, Spec: appsv1.DeploymentSpec{Template: template, Replicas: &replicas}},
		&appsv1.DaemonSet{TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "DaemonSet"}, Spec: appsv1.DaemonSetSpec{Template: template}},
		&batchv1.Job{TypeMeta: metav1.TypeMeta{APIVersion: "batch/v1", Kind: "Job"}, Spec: batchv1.JobSpec{Template: template}},
		&batchv1.CronJob{TypeMeta: metav1.TypeMeta{APIVersion: "batch/v1", Kind: "CronJob"}, Spec: batchv1.CronJobSpec{JobTemplate: batchv1.JobTemplateSpec{Spec: batchv1.JobSpec{Template: template}}}},
		&corev1.Pod{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"}, Spec: template.Spec},
	}
	for _, obj := range objects {
		t.Run(obj.GetObjectKind().GroupVersionKind().Kind, func(t *testing.T) {
			src, err := json.Marshal(obj)
			if err != nil {
				t.Fatal(err)
			}
			got, err := ParseManifest(src, &CostModel{Cluster: &Cluster{NodeCount: 3}})
			if err != nil {
				t.Fatal(err)
			}
			wantReplicas := 1
			switch obj.(type) {
			case *appsv1.StatefulSet, *appsv1.Deployment, *appsv1.DaemonSet:
				wantReplicas = 3
			}
			memory := resource.MustParse("9Gi")
			if got.CPUPerPod != 9000 || got.MemoryPerPod != memory.Value() || got.Replicas != wantReplicas || got.TotalCPU() != 9000*int64(wantReplicas) || got.TotalMemory() != memory.Value()*int64(wantReplicas) {
				t.Fatalf("unexpected requirements: %+v", got)
			}
			if _, ok := obj.(*appsv1.StatefulSet); ok {
				storage := resource.MustParse("32Gi")
				if got.PersistentVolumePerPod != storage.Value() || got.TotalPersistentVolume() != 3*storage.Value() {
					t.Fatalf("unexpected PVC requirements: %+v", got)
				}
			}
		})
	}
}
