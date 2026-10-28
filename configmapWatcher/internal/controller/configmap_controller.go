/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const configDataKey = "service-configs"

// ConfigMapPayload is the JSON document expected in the ConfigMap data.
type ConfigMapPayload struct {
	ServiceConfigs ServiceConfig `json:"serviceConfigs"`
}

// ServiceConfig is the nested service configuration inside ConfigMapPayload.
type ServiceConfig struct {
	WorkerCount int `json:"workerCount"`
	Service     struct {
		Port int `json:"port"`
	} `json:"service"`
}

// ConfigMapReconciler reconciles a ConfigMap object
type ConfigMapReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	TargetNamespace  string
	TargetDeployment string
	ConfigMapName    string
}

// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=core,resources=configmaps/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ConfigMap object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.25.0/pkg/reconcile
func (r *ConfigMapReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = logf.FromContext(ctx)

	if r.ConfigMapName != "" && req.Name != r.ConfigMapName {
		return ctrl.Result{}, nil
	}

	configs := corev1.ConfigMap{}
	key := client.ObjectKey{Namespace: req.Namespace, Name: req.Name}
	if r.ConfigMapName != "" {
		key.Name = r.ConfigMapName
	}
	if err := r.Client.Get(ctx, key, &configs); err != nil {
		return ctrl.Result{}, err
	}

	cfg, err := parseServiceConfig(&configs)
	if err != nil {
		r.Recorder.Event(&configs, corev1.EventTypeWarning, "InvalidConfig", err.Error())
		logf.FromContext(ctx).Info("invalid configmap data",
			"configmap", req.Name,
			"namespace", req.Namespace,
			"reason", err.Error())
		return ctrl.Result{}, nil
	}

	workerCount := cfg.WorkerCount

	deployment := appsv1.Deployment{}
	if err := r.Client.Get(ctx, client.ObjectKey{
		Name:      r.TargetDeployment,
		Namespace: r.TargetNamespace,
	}, &deployment); err != nil {
		r.Recorder.Event(&configs, corev1.EventTypeWarning, "TargetDeploymentUnavailable",
			fmt.Sprintf("cannot get deployment %s/%s: %v", r.TargetNamespace, r.TargetDeployment, err))
		return ctrl.Result{
			Requeue:      true,
			RequeueAfter: 1 * time.Minute,
		}, err
	}

	currentCount := int(*deployment.Spec.Replicas)

	logf.FromContext(ctx).Info("Comparing replica count",
		"desired", workerCount,
		"current", currentCount,
		"deployment", r.TargetDeployment,
		"namespace", r.TargetNamespace,
	)

	if workerCount == currentCount {
		logf.FromContext(ctx).Info("Replica count matches, no action needed",
			"count", currentCount)
		return ctrl.Result{}, nil
	}

	logf.FromContext(ctx).Info("Scaling deployment",
		"from", currentCount,
		"to", workerCount,
		"deployment", r.TargetDeployment,
	)
	*deployment.Spec.Replicas = int32(workerCount)
	if err := r.Client.Update(ctx, &deployment); err != nil {
		r.Recorder.Event(&configs, corev1.EventTypeWarning, "ScaleFailed",
			fmt.Sprintf("cannot scale deployment %s/%s from %d to %d: %v",
				r.TargetNamespace, r.TargetDeployment, currentCount, workerCount, err))
		return ctrl.Result{
			Requeue:      true,
			RequeueAfter: 1 * time.Minute,
		}, err
	}

	r.Recorder.Event(&configs, corev1.EventTypeNormal, "Scaled",
		fmt.Sprintf("scaled deployment %s/%s from %d to %d",
			r.TargetNamespace, r.TargetDeployment, currentCount, workerCount))

	return ctrl.Result{}, nil
}

// parseServiceConfig decodes the JSON payload stored under configDataKey.
func parseServiceConfig(configs *corev1.ConfigMap) (ServiceConfig, error) {
	raw, ok := configs.Data[configDataKey]
	if !ok {
		return ServiceConfig{}, fmt.Errorf("configmap data has no %q key", configDataKey)
	}

	var payload ConfigMapPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return ServiceConfig{}, fmt.Errorf("%s: invalid JSON: %w", configDataKey, err)
	}

	if payload.ServiceConfigs.WorkerCount < 0 {
		return ServiceConfig{}, fmt.Errorf("%s: workerCount must be >= 0, got %d",
			configDataKey, payload.ServiceConfigs.WorkerCount)
	}

	return payload.ServiceConfigs, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ConfigMapReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}).
		Named("configmap").
		Complete(r)
}
